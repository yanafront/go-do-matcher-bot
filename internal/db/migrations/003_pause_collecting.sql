ALTER TABLE users ADD COLUMN IF NOT EXISTS search_active BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS hiring_paused BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE vacancies ADD COLUMN IF NOT EXISTS collecting BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS idx_vacancies_collecting ON vacancies (status, collecting) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS idx_users_search_active ON users (role, search_active) WHERE role = 'candidate';
