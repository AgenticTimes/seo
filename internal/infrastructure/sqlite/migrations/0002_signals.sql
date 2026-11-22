-- Derived analysis snapshot for each keyword (BR-12..BR-19).
ALTER TABLE keywords ADD COLUMN signals TEXT NOT NULL DEFAULT '{}';
