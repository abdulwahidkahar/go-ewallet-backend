DROP INDEX IF EXISTS idx_transfers_status_created_at;

ALTER TABLE transfers
    DROP CONSTRAINT IF EXISTS chk_transfers_amount_positive,
    DROP CONSTRAINT IF EXISTS chk_transfers_status,
    DROP CONSTRAINT IF EXISTS fk_transfers_created_by_user,
    DROP CONSTRAINT IF EXISTS fk_transfers_idempotency_key,
    DROP CONSTRAINT IF EXISTS uq_transfers_reference_id;

ALTER TABLE transfers
    DROP COLUMN IF EXISTS created_by_user_id,
    DROP COLUMN IF EXISTS recipient_balance_after,
    DROP COLUMN IF EXISTS recipient_balance_before,
    DROP COLUMN IF EXISTS sender_balance_after,
    DROP COLUMN IF EXISTS sender_balance_before,
    DROP COLUMN IF EXISTS idempotency_key_id,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS reference_id;
