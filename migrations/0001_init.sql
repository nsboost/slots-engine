-- Ledger schema. Mirrors internal/ledger's double-entry model exactly —
-- if you change the Go types, update this file in the same commit.

CREATE TABLE IF NOT EXISTS transactions (
    id          TEXT PRIMARY KEY,       -- caller-supplied idempotency key
    type        TEXT NOT NULL,
    ref         TEXT NOT NULL DEFAULT '',
    payload     TEXT NOT NULL DEFAULT '', -- opaque caller blob (e.g. serialized SpinResult), returned verbatim on replay
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS entries (
    id              BIGSERIAL PRIMARY KEY,
    transaction_id  TEXT NOT NULL REFERENCES transactions(id),
    player_id       TEXT NOT NULL DEFAULT '',   -- '' for house/system accounts
    account_kind    TEXT NOT NULL,
    amount          BIGINT NOT NULL             -- positive=credit, negative=debit
);

CREATE INDEX IF NOT EXISTS idx_entries_transaction_id ON entries(transaction_id);
CREATE INDEX IF NOT EXISTS idx_entries_player_account ON entries(player_id, account_kind);

-- Running balance per account, maintained transactionally alongside each
-- entry insert (see PGStore.Apply). Reading this table is O(1) instead of
-- summing the full entries history on every balance check.
CREATE TABLE IF NOT EXISTS balances (
    player_id     TEXT NOT NULL DEFAULT '',
    account_kind  TEXT NOT NULL,
    balance       BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (player_id, account_kind)
);

-- Invariant check, useful as a periodic reconciliation job: every
-- transaction's entries must sum to zero. This query should ALWAYS return
-- zero rows; if it doesn't, something bypassed the ledger package and
-- wrote directly to these tables.
--
-- SELECT transaction_id, SUM(amount) AS imbalance
-- FROM entries
-- GROUP BY transaction_id
-- HAVING SUM(amount) <> 0;
