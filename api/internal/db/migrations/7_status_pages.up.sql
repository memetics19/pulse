CREATE TABLE status_pages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    domain     TEXT NOT NULL DEFAULT '' UNIQUE,
    title      TEXT NOT NULL DEFAULT 'Status',
    is_default INTEGER NOT NULL DEFAULT 0,
    published  INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE status_page_groups (
    status_page_id INTEGER NOT NULL REFERENCES status_pages(id) ON DELETE CASCADE,
    group_id       INTEGER NOT NULL,
    PRIMARY KEY (status_page_id, group_id)
);
INSERT INTO status_pages (domain, title, is_default, published) VALUES ('', 'Status', 1, 1);
