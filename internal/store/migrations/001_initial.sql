CREATE TABLE meta (name TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO meta VALUES ('schema_version', '1');
CREATE TABLE receipts (
 id TEXT PRIMARY KEY, started_at TEXT NOT NULL, upstream TEXT NOT NULL, api TEXT NOT NULL,
 requested_model TEXT, reported_model TEXT, provider TEXT, tag TEXT, status INTEGER,
 input_tokens INTEGER, output_tokens INTEGER, cost_amount TEXT, cost_source TEXT NOT NULL,
 ttfb_ms INTEGER, total_ms INTEGER NOT NULL, has_findings INTEGER NOT NULL, envelope BLOB NOT NULL
);
CREATE INDEX receipts_started ON receipts(started_at);
CREATE INDEX receipts_tag ON receipts(tag);
CREATE INDEX receipts_model ON receipts(requested_model);
CREATE INDEX receipts_upstream ON receipts(upstream);
CREATE TABLE supplements (receipt_id TEXT NOT NULL REFERENCES receipts(id) ON DELETE CASCADE, envelope BLOB NOT NULL);
CREATE TABLE contents (receipt_id TEXT PRIMARY KEY REFERENCES receipts(id) ON DELETE CASCADE, request_body BLOB, response_text BLOB, truncated INTEGER NOT NULL);
