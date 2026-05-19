# Production-Minded Wallet Schema

Dokumen ini menjelaskan target schema berikutnya untuk repo ini tanpa memutus flow lama secara langsung.

## Prinsip

- `wallets.balance` tetap dipakai sebagai current balance snapshot untuk query cepat.
- Audit trail utama pindah ke `ledger_entries`, bukan lagi bergantung pada `wallets.balance`.
- `top_up_history` dianggap legacy. Flow baru top up memakai `top_up_orders`.
- Idempotency disimpan di tabel terpisah `idempotency_keys` agar bisa dipakai lintas use case.

## Tabel Baru

### `idempotency_keys`

Dipakai untuk mencegah request yang sama diproses dua kali.

Kolom penting:

- `user_id`: pemilik request.
- `scope`: contoh `wallet.transfer` atau `wallet.topup`.
- `idempotency_key`: key dari client.
- `request_hash`: hash payload untuk mendeteksi reuse key dengan body berbeda.
- `status`: `processing`, `completed`, `failed`.
- `resource_type` dan `resource_id`: pointer ke record bisnis yang dihasilkan.

Constraint utama:

- unique `(user_id, scope, idempotency_key)`

### `top_up_orders`

Flow top up berubah dari "langsung tambah saldo" menjadi "buat order lalu konfirmasi".

Kolom penting:

- `reference_id`: reference internal yang stabil.
- `idempotency_key_id`: relasi ke request idempotent.
- `status`: `pending`, `success`, `failed`.
- `payment_channel`: placeholder untuk channel pembayaran.
- `external_reference`: reference dari payment provider atau simulasi callback.
- `balance_before` dan `balance_after`: snapshot saat top up benar-benar sukses.

Flow yang dituju:

1. Create top up order dengan status `pending`
2. Confirm top up atau simulasi callback
3. Jika sukses, update `wallets.balance` dan tulis `ledger_entries`

### `ledger_entries`

Ini adalah audit trail immutable untuk semua pergerakan saldo.

Kolom penting:

- `wallet_id`
- `reference_id`
- `transaction_type`
- `direction`
- `amount`
- `balance_before`
- `balance_after`
- `transfer_id` atau `top_up_order_id`

Aturan penting:

- record tidak di-update setelah dibuat
- 1 transfer menghasilkan 2 entry: debit pengirim dan credit penerima
- 1 top up sukses menghasilkan 1 entry: credit

## Perubahan ke `transfers`

Tabel `transfers` tetap dipakai, tapi sekarang menjadi business record yang lebih lengkap.

Kolom tambahan:

- `reference_id`
- `status`
- `description`
- `idempotency_key_id`
- `sender_balance_before`
- `sender_balance_after`
- `recipient_balance_before`
- `recipient_balance_after`
- `created_by_user_id`

Catatan:

- snapshot before/after di `transfers` berguna untuk cepat dibaca pada level bisnis
- audit final tetap ada di `ledger_entries`

## Urutan Implementasi yang Disarankan

1. Implement repository dan service untuk `idempotency_keys`
2. Ubah top up menjadi `create` dan `confirm`
3. Ubah transfer agar menyimpan `reference_id`, snapshot balance, dan ledger entry
4. Tambahkan endpoint history dari `top_up_orders` dan `ledger_entries`
5. Tambahkan test business rule dan concurrent transfer
