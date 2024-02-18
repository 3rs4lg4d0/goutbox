-- Table to store events using the outbox pattern
CREATE TABLE outbox (
    id             UUID                     PRIMARY KEY, -- Unique identifier for each event
    aggregate_type VARCHAR(255)             NOT NULL, -- Type of aggregate associated with the event
    aggregate_id   VARCHAR(255)             NOT NULL, -- Identifier of the specific aggregate
    event_type     VARCHAR(255)             NOT NULL, -- Type of the event
    payload        BYTEA                    NOT NULL, -- Binary data representing the event payload
    created_at     TIMESTAMP with time zone NOT NULL     DEFAULT CURRENT_TIMESTAMP -- Timestamp of when the event was created
);

-- Index on the created_at column for optimizing queries based on creation time
CREATE INDEX idx_outbox_created_at ON outbox (created_at);

-- Table to manage locks for preventing concurrent processing of outbox events using optimistic locking
CREATE TABLE outbox_lock (
    id           INTEGER                   PRIMARY KEY, -- Unique identifier for the lock
    locked       BOOLEAN                   NOT NULL DEFAULT false, -- Indicates if the lock is currently active
    locked_by    UUID, -- Unique identifier of the locker dispatcher
    locked_at    TIMESTAMP with time zone, -- Timestamp of when the lock was acquired
    locked_until TIMESTAMP with time zone, -- Timestamp indicating until when the lock is valid
    version      BIGINT                    NOT NULL -- Version number for optimistic locking
);

-- Initial record for the outbox_lock table to signify no active locks
INSERT INTO outbox_lock (id, locked, locked_at, locked_until, version)
VALUES (1, false, null, null, 1);

-- Table to manage subscriptions of dispatchers
CREATE TABLE outbox_dispatcher_subscription (
    id            INTEGER                   PRIMARY KEY, -- Unique identifier for each dispatcher subscription
    dispatcher_id UUID                      NOT NULL, -- Unique identifier for each dispatcher
    alive_at      TIMESTAMP with time zone, -- Timestamp indicating the last time the dispatcher was alive
    version      BIGINT                    NOT NULL -- Version number for optimistic locking
);

-- Index on the dispatcher_id column for optimizing queries
CREATE INDEX idx_ods_dispatcher_id ON outbox_dispatcher_subscription (dispatcher_id);