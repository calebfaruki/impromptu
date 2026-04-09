CREATE TABLE authors (
    id           INTEGER PRIMARY KEY,
    username     TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    avatar_url   TEXT NOT NULL DEFAULT '',
    profile_url  TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE prompts (
    id          INTEGER PRIMARY KEY,
    author_id   INTEGER NOT NULL REFERENCES authors(id),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(author_id, name)
);

CREATE TABLE versions (
    id         INTEGER PRIMARY KEY,
    prompt_id  INTEGER NOT NULL REFERENCES prompts(id),
    version    TEXT NOT NULL,
    digest     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(prompt_id, version)
);

CREATE VIRTUAL TABLE prompts_fts USING fts5(
    name,
    description,
    author,
    content='',
    tokenize='unicode61'
);
