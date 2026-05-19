ALTER TABLE transfers
    ADD COLUMN reference_id VARCHAR(100),
    ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'success',
    ADD COLUMN description TEXT,
    ADD COLUMN idempotency_key_id INT UNIQUE,
    ADD COLUMN sender_balance_before BIGINT,
    ADD COLUMN sender_balance_after BIGINT,
    ADD COLUMN recipient_balance_before BIGINT,
    ADD COLUMN recipient_balance_after BIGINT,
    ADD COLUMN created_by_user_id INT;

UPDATE transfers
SET reference_id = CONCAT('TRF-', LPAD(id::text, 10, '0'))
WHERE reference_id IS NULL;

ALTER TABLE transfers
    ALTER COLUMN reference_id SET NOT NULL;

ALTER TABLE transfers
    ADD CONSTRAINT uq_transfers_reference_id
        UNIQUE (reference_id),
    ADD CONSTRAINT fk_transfers_idempotency_key
        FOREIGN KEY (idempotency_key_id) REFERENCES idempotency_keys(id) ON DELETE SET NULL,
    ADD CONSTRAINT fk_transfers_created_by_user
        FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT chk_transfers_status
        CHECK (status IN ('pending', 'success', 'failed')),
    ADD CONSTRAINT chk_transfers_amount_positive
        CHECK (amount > 0);

CREATE INDEX idx_transfers_status_created_at
    ON transfers (status, created_at DESC);
