PRAGMA foreign_keys = ON;

CREATE TABLE orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
);
CREATE TABLE memberships (
  orbit_id INTEGER NOT NULL REFERENCES orbits(id)
);
CREATE TABLE auxiliary(id INTEGER PRIMARY KEY, value TEXT);
CREATE TABLE audit(value TEXT);
CREATE INDEX orbits_title_idx ON orbits(title);
CREATE TRIGGER orbits_insert_audit AFTER INSERT ON orbits
BEGIN
  INSERT INTO audit(value) VALUES ('orbit');
END;
CREATE VIEW orbits_view AS SELECT id, title FROM orbits;
CREATE VIEW auxiliary_view AS SELECT id, value FROM auxiliary;
CREATE TRIGGER auxiliary_orbit_trigger AFTER INSERT ON auxiliary
BEGIN
  INSERT INTO audit(value) SELECT title FROM orbits ORDER BY id LIMIT 1;
END;
CREATE TRIGGER auxiliary_plain_trigger AFTER UPDATE ON auxiliary
BEGIN
  INSERT INTO audit(value) VALUES ('aux');
END;

INSERT INTO orbits(title, created_at, status) VALUES ('one', 1, 'active');
INSERT INTO memberships(orbit_id) VALUES (1);

PRAGMA foreign_keys = OFF;
BEGIN IMMEDIATE;
DROP VIEW orbits_view;
DROP VIEW auxiliary_view;
DROP TRIGGER auxiliary_orbit_trigger;
DROP TRIGGER auxiliary_plain_trigger;
CREATE TABLE orbits_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK(status IN ('active', 'disabled'))
);
INSERT INTO orbits_new
SELECT id, title, takeover_policy, voice_default, max_pulsars, max_members,
       created_at, status
FROM orbits;
DROP TABLE orbits;
ALTER TABLE orbits_new RENAME TO orbits;
CREATE INDEX orbits_title_idx ON orbits(title);
CREATE TRIGGER orbits_insert_audit AFTER INSERT ON orbits
BEGIN
  INSERT INTO audit(value) VALUES ('orbit');
END;
CREATE VIEW orbits_view AS SELECT id, title FROM orbits;
CREATE VIEW auxiliary_view AS SELECT id, value FROM auxiliary;
CREATE TRIGGER auxiliary_orbit_trigger AFTER INSERT ON auxiliary
BEGIN
  INSERT INTO audit(value) SELECT title FROM orbits ORDER BY id LIMIT 1;
END;
CREATE TRIGGER auxiliary_plain_trigger AFTER UPDATE ON auxiliary
BEGIN
  INSERT INTO audit(value) VALUES ('aux');
END;
COMMIT;
PRAGMA foreign_keys = ON;

SELECT 'objects', group_concat(type || ':' || name, ',')
FROM (
  SELECT type, name FROM sqlite_master
  WHERE name IN (
    'orbits_title_idx', 'orbits_insert_audit', 'orbits_view',
    'auxiliary_view', 'auxiliary_orbit_trigger', 'auxiliary_plain_trigger'
  )
  ORDER BY type, name
);
SELECT 'foreign_keys', foreign_keys FROM pragma_foreign_keys;
SELECT 'fk_violations', count(*) FROM pragma_foreign_key_check;
SELECT 'status_check', count(*)
FROM pragma_table_info('orbits')
WHERE name = 'status' AND "notnull" = 1;
INSERT OR IGNORE INTO orbits(title, created_at, status)
VALUES ('invalid', 2, 'bogus');
SELECT 'bogus_rows', count(*) FROM orbits WHERE status = 'bogus';
