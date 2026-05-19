CREATE TABLE idempotency_keys (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL,
    scope VARCHAR(50) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'processing',
    resource_type VARCHAR(50),
    resource_id INT,
    locked_at TIMESTAMP WITH TIME ZONE,
    last_seen_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_idempotency_keys_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT uq_idempotency_keys_user_scope_key
        UNIQUE (user_id, scope, idempotency_key),
    CONSTRAINT chk_idempotency_keys_status
        CHECK (status IN ('processing', 'completed', 'failed'))
);

CREATE INDEX idx_idempotency_keys_lookup
    ON idempotency_keys (user_id, scope, idempotency_key);

CREATE INDEX idx_idempotency_keys_status
    ON idempotency_keys (status);
