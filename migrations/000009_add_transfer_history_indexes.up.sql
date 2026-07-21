CREATE INDEX idx_transfers_from_wallet_created_at
    ON transfers (from_wallet_id, created_at DESC);

CREATE INDEX idx_transfers_to_wallet_created_at
    ON transfers (to_wallet_id, created_at DESC);
