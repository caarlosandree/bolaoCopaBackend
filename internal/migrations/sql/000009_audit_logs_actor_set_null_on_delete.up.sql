-- Ajusta a FK de audit_logs.actor_user_id para ON DELETE SET NULL,
-- permitindo a exclusão de usuários sem violar a integridade referencial.
-- Os palpites (guesses) já possuem ON DELETE CASCADE em relação a users.

ALTER TABLE audit_logs
    DROP CONSTRAINT IF EXISTS audit_logs_actor_user_id_fkey;

ALTER TABLE audit_logs
    ADD CONSTRAINT audit_logs_actor_user_id_fkey
    FOREIGN KEY (actor_user_id) REFERENCES users(id)
    ON DELETE SET NULL;
