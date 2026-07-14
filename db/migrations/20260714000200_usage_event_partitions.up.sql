CREATE OR REPLACE FUNCTION manage_usage_event_partitions(
    p_days_ahead integer DEFAULT 7,
    p_retention_days integer DEFAULT 90
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    partition_day date;
    partition_name text;
BEGIN
    IF p_days_ahead < 0 OR p_retention_days < 1 THEN
        RAISE EXCEPTION 'invalid usage partition window';
    END IF;

    FOR offset_day IN 0..p_days_ahead LOOP
        partition_day := CURRENT_DATE + offset_day;
        partition_name := format('usage_events_%s', to_char(partition_day, 'YYYYMMDD'));
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF usage_events FOR VALUES FROM (%L) TO (%L)',
            partition_name,
            partition_day,
            partition_day + 1
        );
    END LOOP;

    FOR partition_name IN
        SELECT child.relname
        FROM pg_inherits
        JOIN pg_class parent ON parent.oid = pg_inherits.inhparent
        JOIN pg_class child ON child.oid = pg_inherits.inhrelid
        WHERE parent.relname = 'usage_events'
          AND child.relname ~ '^usage_events_[0-9]{8}$'
    LOOP
        partition_day := to_date(substring(partition_name FROM 14), 'YYYYMMDD');
        IF partition_day < CURRENT_DATE - p_retention_days THEN
            EXECUTE format('DROP TABLE IF EXISTS %I', partition_name);
        END IF;
    END LOOP;
END;
$$;

SELECT manage_usage_event_partitions();
