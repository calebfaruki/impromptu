CREATE TABLE IF NOT EXISTS indexed_prompts (
    id              INTEGER PRIMARY KEY,
    source_url      TEXT NOT NULL,
    digest          TEXT NOT NULL,
    signer_identity TEXT NOT NULL DEFAULT '',
    rekor_log_index INTEGER NOT NULL DEFAULT 0,
    indexed_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(source_url, digest)
);

CREATE VIRTUAL TABLE IF NOT EXISTS indexed_prompts_fts USING fts5(
    source_url,
    signer_identity,
    content='',
    tokenize='unicode61'
);
