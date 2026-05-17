# Go Payment Service

REST API layanan pembayaran berbasis Go dengan fitur autentikasi JWT, profile retrieval, top up wallet, cek saldo, dan transfer saldo.

## Tech Stack

- Go
- Gin
- PostgreSQL
- Redis
- Docker Compose
- golang-migrate

## Endpoints

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `POST` | `/register` | No | Register user baru |
| `POST` | `/login` | No | Login dan generate JWT |
| `GET` | `/health` | No | Health check service |
| `POST` | `/api/logout` | Yes | Invalidate JWT dengan blacklist Redis |
| `GET` | `/api/profile` | Yes | Ambil profile user dari JWT context |
| `POST` | `/api/wallet/topup` | Yes | Tambah saldo wallet user login |
| `GET` | `/api/wallet/topup/history` | Yes | Ambil riwayat top up user login |
| `GET` | `/api/wallet/balance` | Yes | Ambil saldo wallet user login |
| `POST` | `/api/wallet/transfer` | Yes | Transfer saldo ke wallet lain |
| `GET` | `/api/wallet/transfer` | Yes | Ambil riwayat transfer user login |

Untuk endpoint yang butuh auth, kirim header:

```http
Authorization: Bearer <jwt-token>
```

## Menjalankan Project

Prasyarat:

- Docker dan Docker Compose tersedia
- CLI `migrate` dari `golang-migrate` sudah terpasang

Jalankan service:

```bash
docker compose up --build
```

Setelah container PostgreSQL siap, jalankan migration:

```bash
migrate -path migrations -database "postgres://[DB_USER]:[DB_PASSWORD]@localhost:5432/[DB_NAME]?sslmode=disable" up
```

API akan berjalan di:

```text
http://localhost:9090
```

Migration terbaru menambahkan tabel `top_up_history`, jadi pastikan file migration `000004_create_top_up_history_table.up.sql` ikut dijalankan sebelum mencoba endpoint riwayat top up.

## Contoh Flow Wallet

Catatan:

- Semua endpoint `/api/*` butuh header `Authorization: Bearer <jwt-token>`
- Pada endpoint transfer, field `to_wallet_id` adalah `id` wallet tujuan, bukan `user_id`

### 1. Register

Request:

```http
POST /register
Content-Type: application/json

{
  "email": "user1@example.com",
  "password": "secret123"
}
```

Response:

```json
{
  "message": "User registered successfully",
  "email": "user1@example.com"
}
```

### 2. Login

Request:

```http
POST /login
Content-Type: application/json

{
  "email": "user1@example.com",
  "password": "secret123"
}
```

Response:

```json
{
  "message": "Login successful",
  "token": "<jwt-token>"
}
```

### 3. Cek Balance

Request:

```http
GET /api/wallet/balance
Authorization: Bearer <jwt-token>
```

Response:

```json
{
  "wallet": {
    "id": 1,
    "user_id": 1,
    "balance": 0,
    "currency": "IDR",
    "created_at": "2026-05-17T10:00:00Z",
    "updated_at": "2026-05-17T10:00:00Z"
  }
}
```

### 4. Top Up

Request:

```http
POST /api/wallet/topup
Authorization: Bearer <jwt-token>
Content-Type: application/json

{
  "amount": 50000
}
```

Response:

```json
{
  "message": "Wallet topped up successfully",
  "top_up": 50000
}
```

### 5. Riwayat Top Up

Request:

```http
GET /api/wallet/topup/history
Authorization: Bearer <jwt-token>
```

Response:

```json
{
  "top_ups": [
    {
      "id": 1,
      "wallet_id": 1,
      "amount": 50000,
      "created_at": "2026-05-17T10:05:00Z"
    }
  ]
}
```

### 6. Transfer

Request:

```http
POST /api/wallet/transfer
Authorization: Bearer <jwt-token>
Content-Type: application/json

{
  "to_wallet_id": 2,
  "amount": 20000
}
```

Response:

```json
{
  "message": "Transfer successful",
  "transfer": {
    "id": 1,
    "from_wallet_id": 1,
    "to_wallet_id": 2,
    "amount": 20000,
    "created_at": "2026-05-17 10:10:00+00"
  }
}
```

### 7. Riwayat Transfer

Request:

```http
GET /api/wallet/transfer
Authorization: Bearer <jwt-token>
```

Response:

```json
{
  "transfers": [
    {
      "id": 1,
      "from_wallet_id": 1,
      "to_wallet_id": 2,
      "amount": 20000,
      "created_at": "2026-05-17T10:10:00Z"
    }
  ]
}
```

## Urutan Testing Manual

1. Jalankan `docker compose up --build`.
2. Jalankan semua migration dengan `migrate ... up`.
3. Hit endpoint `GET /health` untuk memastikan service hidup.
4. Register dua user agar masing-masing punya wallet otomatis.
5. Login sebagai user pertama dan simpan JWT.
6. Cek `GET /api/wallet/balance` untuk melihat `wallet.id` milik user pertama.
7. Login sebagai user kedua lalu cek balance untuk mendapatkan `wallet.id` user kedua.
8. Top up saldo user pertama.
9. Cek `GET /api/wallet/topup/history` untuk memastikan history tercatat.
10. Transfer dari user pertama ke `wallet.id` user kedua.
11. Cek `GET /api/wallet/transfer` untuk memastikan history transfer tercatat.
12. Cek ulang balance kedua user untuk memastikan saldo berubah sesuai transaksi.

## Error Umum

- `401 Unauthorized`: token JWT tidak dikirim, tidak valid, atau sudah di-blacklist saat logout.
- `404 Wallet not found`: wallet user belum ada atau data user/wallet di database tidak konsisten.
- `400 amount must be greater than 0`: nominal top up atau transfer harus lebih besar dari nol.
- `400 Insufficient balance`: saldo pengirim tidak cukup untuk transfer.
- `404 Recipient wallet not found`: `to_wallet_id` tidak ada di tabel `wallets`.

## Keputusan Teknis

### JWT blacklist memakai Redis, bukan PostgreSQL

Logout tidak menghapus JWT yang sudah diterbitkan. Karena itu, token yang sudah logout harus dicek pada setiap request terproteksi.

Redis dipakai untuk blacklist karena:

- in-memory lookup lebih cepat untuk validasi per-request
- cocok untuk data token invalidation yang sifatnya sementara
- beban baca tinggi tidak membebani database utama

PostgreSQL tidak dipakai untuk blacklist token karena:

- query disk-based lebih mahal untuk request yang frekuensinya tinggi
- akan lebih cepat menjadi bottleneck saat jumlah request dan token bertambah
- database relasional sebaiknya fokus ke data bisnis utama, bukan cache-like lookup berulang

### UNIQUE constraint wajib ada di database, bukan hanya di kode

Validasi email unik di application layer saja tidak cukup. Dua request paralel bisa lolos pengecekan kode pada waktu yang hampir sama lalu mencoba insert data yang sama.

Karena itu, constraint `UNIQUE` disimpan di tabel `users` sebagai last line of defense. Application layer tetap menangani error duplicate dengan response yang jelas, tetapi integritas final tetap dijaga database.

### Balance memakai int64 dan transfer memakai FOR UPDATE

Saldo disimpan sebagai `int64`, bukan `float64`, karena nilai uang tidak boleh terkena error pembulatan floating-point yang bisa membuat hasil top up, debit, atau transfer menjadi tidak presisi; untuk transaksi, query `FOR UPDATE` dipakai saat membaca wallet pengirim dan penerima agar kedua row terkunci di dalam satu transaksi, sehingga request paralel tidak membaca balance lama, tidak saling menimpa update, dan risiko double spending bisa ditekan.
