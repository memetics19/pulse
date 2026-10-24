-- The up migration only rewrites timestamps into UTC. The original offsets are
-- not kept, and the UTC text is valid for every older version, so there is
-- nothing to undo.
SELECT 1;
