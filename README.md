# Go E-Wallet Backend Service

REST API e-wallet berbasis Go dengan fitur:

- JWT authentication
- wallet balance snapshot
- top up order lifecycle (`pending` -> `success`)
- idempotent transfer dan top up request
- immutable ledger entries untuk audit trail

## Tech Stack

- Go
- Gin
- PostgreSQL
- Redis
- Docker Compose
- golang-migrate

## Gambaran Arsitektur

- `wallets.balance` dipakai sebagai current balance snapshot untuk query cepat.
- `top_up_orders` menyimpan lifecycle top up.
- `transfers` menyimpan business record transfer.
- `ledger_entries` menyimpan jejak debit/credit yang immutable.
- `idempotency_keys` mencegah request ganda memproses uang dua kali.

Detail schema ada di [docs/production-wallet-schema.md](/Users/wahid/Documents/Development/GOLANG/auth-api/docs/production-wallet-schema.md:1).

### Entity Relationship Diagram

![E-Wallet ERD](docs/e-wallet.svg)

### Idempotency Flow

![Idempotency Flow](docs/idempotency_flow.gif)

## Endpoints

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `POST` | `/register` | No | Register user baru |
| `POST` | `/login` | No | Login dan generate JWT |
| `GET` | `/health` | No | Health check service |
| `POST` | `/api/logout` | Yes | Invalidate JWT dengan blacklist Redis |
| `GET` | `/api/profile` | Yes | Ambil profile user dari JWT context |
| `GET` | `/api/wallet/balance` | Yes | Ambil saldo wallet user login |
| `POST` | `/api/wallet/topup` | Yes | Alias untuk create top up order |
| `POST` | `/api/wallet/topups` | Yes | Create top up order baru |
| `POST` | `/api/wallet/topups/:reference_id/confirm` | Yes | Confirm top up order |
| `GET` | `/api/wallet/topups` | Yes | Ambil riwayat top up order user login |
| `GET` | `/api/wallet/topup/history` | Yes | Alias history top up order |
| `GET` | `/api/wallet/ledger` | Yes | Ambil riwayat ledger wallet user login |
| `POST` | `/api/wallet/transfer` | Yes | Transfer saldo ke wallet lain |
| `GET` | `/api/wallet/transfer` | Yes | Ambil riwayat transfer user login |

## Headers Penting

Untuk endpoint yang butuh auth:

```http
Authorization: Bearer <jwt-token>
```

Untuk endpoint yang memindahkan uang atau membuat order transaksi:

```http
Idempotency-Key: <unique-client-generated-key>
```

Endpoint yang saat ini mewajibkan `Idempotency-Key`:

- `POST /api/wallet/topup`
- `POST /api/wallet/topups`
- `POST /api/wallet/topups/:reference_id/confirm`
- `POST /api/wallet/transfer`

## Menjalankan Project

Prasyarat:

- Docker dan Docker Compose tersedia
- CLI `migrate` dari `golang-migrate` sudah terpasang

Jalankan service:

```bash
docker compose up --build
```

Setelah container PostgreSQL siap, jalankan semua migration:

```bash
migrate -path migrations -database "postgres://[DB_USER]:[DB_PASSWORD]@localhost:5432/[DB_NAME]?sslmode=disable" up
```

API akan berjalan di:

```text
http://localhost:9090
```

Schema terbaru menambah `idempotency_keys`, `top_up_orders`, `ledger_entries`, dan field baru di `transfers`, jadi pastikan migration `000005` sampai `000008` ikut dijalankan.

## Contoh Flow

Catatan:

- Semua endpoint `/api/*` butuh header `Authorization: Bearer <jwt-token>`
- Transfer memakai `to_wallet_id`, bukan `user_id`
- Top up sekarang tidak langsung mengubah saldo saat order dibuat

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
    "created_at": "2026-05-19T10:00:00Z",
    "updated_at": "2026-05-19T10:00:00Z"
  }
}
```

### 4. Create Top Up Order

Request:

```http
POST /api/wallet/topups
Authorization: Bearer <jwt-token>
Idempotency-Key: topup-create-user1-001
Content-Type: application/json

{
  "amount": 50000,
  "payment_channel": "manual",
  "description": "Top up pertama"
}
```

Response:

```json
{
  "message": "Top up order created successfully",
  "top_up": {
    "id": 1,
    "wallet_id": 1,
    "reference_id": "TUP-20260519093000-ab12cd34",
    "amount": 50000,
    "status": "pending",
    "payment_channel": "manual",
    "description": "Top up pertama",
    "created_at": "2026-05-19 09:30:00+00",
    "updated_at": "2026-05-19 09:30:00+00"
  }
}
```

### 5. Confirm Top Up Order

Request:

```http
POST /api/wallet/topups/TUP-20260519093000-ab12cd34/confirm
Authorization: Bearer <jwt-token>
Idempotency-Key: topup-confirm-user1-001
Content-Type: application/json

{
  "external_reference": "SIM-VA-0001",
  "description": "Simulated payment confirmation"
}
```

Response:

```json
{
  "message": "Top up confirmed successfully",
  "top_up": {
    "id": 1,
    "wallet_id": 1,
    "reference_id": "TUP-20260519093000-ab12cd34",
    "amount": 50000,
    "status": "success",
    "payment_channel": "manual",
    "external_reference": "SIM-VA-0001",
    "description": "Simulated payment confirmation",
    "balance_before": 0,
    "balance_after": 50000,
    "created_at": "2026-05-19 09:30:00+00",
    "updated_at": "2026-05-19 09:31:10+00",
    "confirmed_at": "2026-05-19 09:31:10+00"
  }
}
```

### 6. Riwayat Top Up Order

Request:

```http
GET /api/wallet/topups
Authorization: Bearer <jwt-token>
```

Response:

```json
{
  "top_ups": [
    {
      "id": 1,
      "wallet_id": 1,
      "reference_id": "TUP-20260519093000-ab12cd34",
      "amount": 50000,
      "status": "success",
      "payment_channel": "manual",
      "external_reference": "SIM-VA-0001",
      "description": "Simulated payment confirmation",
      "balance_before": 0,
      "balance_after": 50000,
      "created_at": "2026-05-19 09:30:00+00",
      "updated_at": "2026-05-19 09:31:10+00",
      "confirmed_at": "2026-05-19 09:31:10+00"
    }
  ]
}
```

### 7. Transfer

Request:

```http
POST /api/wallet/transfer
Authorization: Bearer <jwt-token>
Idempotency-Key: transfer-user1-001
Content-Type: application/json

{
  "to_wallet_id": 2,
  "amount": 20000,
  "description": "Bayar kopi"
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
    "reference_id": "TRF-20260519093500-ef56ab78",
    "status": "success",
    "description": "Bayar kopi",
    "sender_balance_before": 50000,
    "sender_balance_after": 30000,
    "recipient_balance_before": 0,
    "recipient_balance_after": 20000,
    "created_at": "2026-05-19 09:35:00+00"
  }
}
```

### 8. Riwayat Transfer

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
      "reference_id": "TRF-20260519093500-ef56ab78",
      "status": "success",
      "description": "Bayar kopi",
      "created_at": "2026-05-19T09:35:00Z"
    }
  ]
}
```

### 9. Riwayat Ledger

Request:

```http
GET /api/wallet/ledger
Authorization: Bearer <jwt-token>
```

Response:

```json
{
  "ledger_entries": [
    {
      "id": 1,
      "wallet_id": 1,
      "reference_id": "TRF-20260519093500-ef56ab78",
      "transaction_type": "transfer",
      "direction": "debit",
      "amount": 20000,
      "balance_before": 50000,
      "balance_after": 30000,
      "description": "Bayar kopi",
      "transfer_id": 1,
      "created_at": "2026-05-19T09:35:00Z"
    },
    {
      "id": 2,
      "wallet_id": 1,
      "reference_id": "TUP-20260519093000-ab12cd34",
      "transaction_type": "top_up",
      "direction": "credit",
      "amount": 50000,
      "balance_before": 0,
      "balance_after": 50000,
      "description": "Simulated payment confirmation",
      "top_up_order_id": 1,
      "created_at": "2026-05-19T09:31:10Z"
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
6. Login sebagai user kedua dan simpan JWT.
7. Cek `GET /api/wallet/balance` dari kedua user untuk mendapatkan `wallet.id`.
8. Buat top up order user pertama memakai `POST /api/wallet/topups`.
9. Confirm top up order user pertama memakai `POST /api/wallet/topups/:reference_id/confirm`.
10. Cek ulang balance user pertama untuk memastikan saldo bertambah.
11. Transfer dari user pertama ke `wallet.id` user kedua memakai `POST /api/wallet/transfer`.
12. Cek `GET /api/wallet/transfer` dan `GET /api/wallet/topups`.
13. Cek `GET /api/wallet/ledger` untuk memastikan debit dan credit tercatat.
14. Cek ulang balance kedua user untuk memastikan saldo berubah sesuai transaksi.

## Error Umum

- `401 Unauthorized`: token JWT tidak dikirim, tidak valid, atau sudah di-blacklist saat logout.
- `400 idempotency key is required`: header `Idempotency-Key` wajib dikirim untuk endpoint transaksi tertentu.
- `409 idempotency key already used with different payload`: key yang sama dipakai dengan request body berbeda.
- `409 request with this idempotency key is still processing`: request sebelumnya dengan key yang sama masih diproses.
- `409 previous request with this idempotency key failed`: request lama dengan key yang sama pernah gagal.
- `404 Wallet not found`: wallet user belum ada atau data user/wallet tidak konsisten.
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

### Idempotency dipakai untuk request yang memindahkan uang

Request transaksi di mobile bisa terkirim dua kali karena retry, timeout, atau user menekan tombol berulang.

Karena itu, request seperti create top up, confirm top up, dan transfer memakai `Idempotency-Key` agar:

- 1 intent user tidak menghasilkan 2 transaksi
- duplicate request bisa mengembalikan hasil lama
- payload yang berbeda dengan key yang sama bisa ditolak

### Ledger entries dipakai sebagai audit trail immutable

`wallets.balance` hanya dipakai sebagai snapshot saldo saat ini.

Sumber audit utama ada di `ledger_entries` karena:

- setiap pergerakan saldo punya record debit/credit
- ledger tidak di-update ulang setelah dibuat
- jejak uang lebih mudah dijelaskan dan diuji

### Balance memakai int64 dan transfer memakai FOR UPDATE

Saldo disimpan sebagai `int64`, bukan `float64`, agar tidak terkena error pembulatan floating-point. Untuk transfer, query `FOR UPDATE` dipakai saat membaca wallet pengirim dan penerima agar kedua row terkunci di dalam satu transaksi dan risiko double spending bisa ditekan.
