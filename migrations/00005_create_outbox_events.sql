-- +goose Up
CREATE TABLE outbox_events (
    id           UUID        PRIMARY KEY,
    event_type   VARCHAR(50) NOT NULL,
    payload      JSONB       NOT NULL,
    status       VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    retries      INTEGER     NOT NULL DEFAULT 0,
    processed_at TIMESTAMP,
    created_at   TIMESTAMP   NOT NULL DEFAULT now(),

    CONSTRAINT chk_outbox_status CHECK (status IN ('PENDING', 'PROCESSED'))
);

CREATE INDEX idx_outbox_status ON outbox_events(status) WHERE status = 'PENDING';

-- +goose Down
DROP TABLE outbox_events;
