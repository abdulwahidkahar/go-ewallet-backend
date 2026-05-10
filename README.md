# Go Auth API

REST API autentikasi berbasis Go untuk register, login, logout, dan profile retrieval dengan JWT.

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
| `POST` | `/api/logout` | Yes | Invalidate JWT dengan blacklist Redis |
| `GET` | `/api/profile` | Yes | Ambil profile user dari JWT context |

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
migrate -path migrations -database "postgres://postgres:password@localhost:5432/auth_api?sslmode=disable" up
```

API akan berjalan di:

```text
http://localhost:9090
```

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
