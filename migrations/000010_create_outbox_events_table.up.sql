CREATE TABLE outbox_events (
    id            BIGSERIAL    PRIMARY KEY,
    event_type    VARCHAR(100) NOT NULL,
    payload       JSONB        NOT NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'PENDING',
    retry_count   INT          NOT NULL DEFAULT 0,
    last_error    TEXT,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at  TIMESTAMP WITH TIME ZONE,

    CONSTRAINT chk_outbox_events_status
        CHECK (status IN ('PENDING', 'PUBLISHED', 'FAILED'))
);

CREATE INDEX idx_outbox_events_pending
    ON outbox_events (created_at)
    WHERE status = 'PENDING';
