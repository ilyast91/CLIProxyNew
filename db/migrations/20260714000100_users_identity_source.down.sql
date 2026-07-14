DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE identity_source = 'static') THEN
        RAISE EXCEPTION 'cannot rollback users identity_source while static users exist';
    END IF;
END $$;

ALTER TABLE users
    DROP CONSTRAINT users_identity_source_namespace_check,
    DROP CONSTRAINT users_identity_source_check,
    DROP COLUMN identity_source;
