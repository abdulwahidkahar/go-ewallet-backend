CREATE TABLE ledger_entries (
    id SERIAL PRIMARY KEY,
    wallet_id INT NOT NULL,
    reference_id VARCHAR(100) NOT NULL,
    transaction_type VARCHAR(50) NOT NULL,
    direction VARCHAR(10) NOT NULL,
    amount BIGINT NOT NULL,
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    description TEXT,
    transfer_id INT,
    top_up_order_id INT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_ledger_entries_wallet
        FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE,
    CONSTRAINT fk_ledger_entries_transfer
        FOREIGN KEY (transfer_id) REFERENCES transfers(id) ON DELETE SET NULL,
    CONSTRAINT fk_ledger_entries_top_up_order
        FOREIGN KEY (top_up_order_id) REFERENCES top_up_orders(id) ON DELETE SET NULL,
    CONSTRAINT chk_ledger_entries_direction
        CHECK (direction IN ('debit', 'credit')),
    CONSTRAINT chk_ledger_entries_transaction_type
        CHECK (transaction_type IN ('transfer', 'top_up')),
    CONSTRAINT chk_ledger_entries_amount_positive
        CHECK (amount > 0),
    CONSTRAINT chk_ledger_entries_balance_progression
        CHECK (
            (direction = 'debit' AND balance_after = balance_before - amount)
            OR (direction = 'credit' AND balance_after = balance_before + amount)
        ),
    CONSTRAINT chk_ledger_entries_single_source
        CHECK (((transfer_id IS NOT NULL)::int + (top_up_order_id IS NOT NULL)::int) = 1)
);

CREATE UNIQUE INDEX uq_ledger_entries_wallet_reference_direction
    ON ledger_entries (wallet_id, reference_id, direction);

CREATE INDEX idx_ledger_entries_wallet_created_at
    ON ledger_entries (wallet_id, created_at DESC);
