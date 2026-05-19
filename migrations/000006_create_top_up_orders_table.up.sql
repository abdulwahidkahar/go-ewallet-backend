CREATE TABLE top_up_orders (
    id SERIAL PRIMARY KEY,
    wallet_id INT NOT NULL,
    reference_id VARCHAR(100) NOT NULL UNIQUE,
    idempotency_key_id INT UNIQUE,
    amount BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    payment_channel VARCHAR(50) NOT NULL DEFAULT 'manual',
    external_reference VARCHAR(100) UNIQUE,
    description TEXT,
    balance_before BIGINT,
    balance_after BIGINT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    confirmed_at TIMESTAMP WITH TIME ZONE,
    failed_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT fk_top_up_orders_wallet
        FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE,
    CONSTRAINT fk_top_up_orders_idempotency_key
        FOREIGN KEY (idempotency_key_id) REFERENCES idempotency_keys(id) ON DELETE SET NULL,
    CONSTRAINT chk_top_up_orders_status
        CHECK (status IN ('pending', 'success', 'failed')),
    CONSTRAINT chk_top_up_orders_amount_positive
        CHECK (amount > 0),
    CONSTRAINT chk_top_up_orders_balance_snapshot
        CHECK (
            (status = 'pending' AND balance_before IS NULL AND balance_after IS NULL)
            OR (status IN ('success', 'failed'))
        )
);

CREATE INDEX idx_top_up_orders_wallet_status_created_at
    ON top_up_orders (wallet_id, status, created_at DESC);
