-- Reverting the baseline would delete all Results data. Refuse unless the
-- tables are empty; every record belongs to a result, so checking results is
-- sufficient.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM results) THEN
        RAISE EXCEPTION 'refusing to drop non-empty Results tables';
    END IF;
END
$$;

DROP TABLE records;
DROP TABLE results;
