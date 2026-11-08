#!/usr/bin/env python3
"""Run the real Postgres image, the role scripts, and the migrate image, and
check what each database role can and cannot do. Needs a Docker daemon; the
containers, network, and image it makes are removed at the end.

    python3 infra/tests/test-database-roles.py
"""
import os
from pathlib import Path
import re
import subprocess
import unittest
import uuid

ROOT = Path(__file__).resolve().parents[2]
DATABASE_APPS = {'auth': 'oauth', 'calendar': 'calendar', 'status': 'logger',
                 'sprinter': 'sprinter'}
# A table of each schema, to read as each role.
TABLES = {'oauth': 'oauth.operators', 'calendar': 'calendar.calendar_scopes',
          'logger': 'logger.health_checks', 'sprinter': 'sprinter.guards'}
OWNER_PASSWORD = 'owner' + uuid.uuid4().hex
PASSWORDS = {f'{app.upper()}_DB_PASSWORD': f'{app}{uuid.uuid4().hex}' for app in DATABASE_APPS}
PASSWORDS['SPRINTER_READER_PASSWORD'] = 'reader' + uuid.uuid4().hex


def docker(*args, check=True, input=None):
    return subprocess.run(['docker', *args], capture_output=True, text=True, check=check,
                          input=input)


def docker_available():
    try:
        return docker('info', check=False).returncode == 0
    except FileNotFoundError:
        return False


def postgres_image():
    compose = (ROOT / 'infra/docker-compose.yml').read_text()
    return re.search(r'image: (postgres:\S+)', compose).group(1)


@unittest.skipUnless(docker_available(), 'needs a Docker daemon')
class DatabaseRoleTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.name = 'biotron-roles-test-' + uuid.uuid4().hex[:8]
        cls.image = postgres_image()
        # BIOTRON_MIGRATE_IMAGE names an image built already, such as CI's.
        cls.prebuilt = os.environ.get('BIOTRON_MIGRATE_IMAGE', '')
        cls.migrate_image = cls.prebuilt or f'{cls.name}-migrate'
        cls.env = {'POSTGRES_USER': 'biotron', 'POSTGRES_DB': 'biotron',
                   'POSTGRES_PASSWORD': OWNER_PASSWORD, **PASSWORDS}
        docker('network', 'create', cls.name)
        try:
            cls.start_postgres()
            cls.run_roles()
            if not cls.prebuilt:
                docker('build', '-q', '-t', cls.migrate_image, '-f', str(ROOT / 'infra/migrate/Dockerfile'),
                       str(ROOT))
            for app in DATABASE_APPS:
                cls.migrate(app)
        except BaseException:
            cls.tearDownClass()
            raise

    @classmethod
    def tearDownClass(cls):
        docker('rm', '-f', '-v', f'{cls.name}-pg', check=False)
        docker('network', 'rm', cls.name, check=False)
        if not cls.prebuilt:
            docker('image', 'rm', '-f', cls.migrate_image, check=False)

    @classmethod
    def env_args(cls, **extra):
        args = []
        for key, value in {**cls.env, **extra}.items():
            args += ['-e', f'{key}={value}']
        return args

    @classmethod
    def with_init_scripts(cls, container, destination):
        # Copied rather than bind-mounted, so the test also works when the
        # Docker daemon cannot see this checkout's path.
        docker('cp', str(ROOT / 'infra/postgres/init') + '/.', f'{container}:{destination}')

    @classmethod
    def start_postgres(cls):
        container = f'{cls.name}-pg'
        docker('create', '--name', container, '--network', cls.name, '--network-alias', 'postgres',
               *cls.env_args(), cls.image)
        cls.with_init_scripts(container, '/docker-entrypoint-initdb.d')
        docker('start', container)
        cls.address = docker('inspect', '--format',
                             '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}',
                             container).stdout.strip()
        # Ready over TCP only once the init scripts are done, as the
        # healthcheck in infra/docker-compose.yml asks.
        for _ in range(120):
            if docker('exec', container, 'pg_isready', '-q', '-h', '127.0.0.1', '-U', 'biotron',
                      check=False).returncode == 0:
                return
            subprocess.run(['sleep', '1'], check=True)
        raise AssertionError(docker('logs', container, check=False).stderr)

    @classmethod
    def run_roles(cls, **extra):
        """What the postgres-roles service does on every start."""
        container = f'{cls.name}-roles-{uuid.uuid4().hex[:6]}'
        docker('create', '--name', container, '--network', cls.name,
               *cls.env_args(PGHOST='postgres', PGPASSWORD=OWNER_PASSWORD, **extra),
               '--entrypoint', 'bash', cls.image, '-ec',
               'for script in /init/*.sh; do bash "$script"; done')
        try:
            cls.with_init_scripts(container, '/init')
            result = docker('start', '-a', container, check=False)
            if result.returncode != 0:
                raise AssertionError(result.stdout + result.stderr)
        finally:
            docker('rm', '-f', container, check=False)

    @classmethod
    def migrate(cls, app, *args, check=True):
        url = f'postgresql://biotron:{OWNER_PASSWORD}@postgres:5432/biotron?schema={DATABASE_APPS.get(app, "x")}'
        return docker('run', '--rm', '--network', cls.name, '-e', f'DATABASE_URL={url}',
                      cls.migrate_image, app, *args, check=check)

    def sql(self, role, password, statement):
        """Runs one statement as a role, with password auth. The image trusts
        connections from 127.0.0.1, so it connects to the container's own
        address on the test network, as an app does."""
        return docker('exec', '-e', f'PGPASSWORD={password}', f'{self.name}-pg', 'psql', '-h',
                      self.address, '-U', role, '-d', 'biotron', '-v', 'ON_ERROR_STOP=1', '-XAtc',
                      statement, check=False)

    def as_app(self, app, statement):
        return self.sql(f'{app}_app', PASSWORDS[f'{app.upper()}_DB_PASSWORD'], statement)

    def as_reader(self, statement):
        return self.sql('sprinter_reader', PASSWORDS['SPRINTER_READER_PASSWORD'], statement)

    def assertAllowed(self, result):
        self.assertEqual(result.returncode, 0, result.stderr)

    def assertRefused(self, result, reason='permission denied'):
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn(reason, result.stderr)

    def test_each_app_reads_and_writes_only_its_own_schema(self):
        for app, schema in DATABASE_APPS.items():
            with self.subTest(app=app):
                self.assertAllowed(self.as_app(app, f'SELECT count(*) FROM {TABLES[schema]}'))
                for other, table in TABLES.items():
                    if other != schema:
                        self.assertRefused(self.as_app(app, f'SELECT count(*) FROM {table}'))

    def test_an_app_writes_rows_but_changes_no_schema(self):
        self.assertAllowed(self.as_app(
            'status', "INSERT INTO logger.health_checks (service, ok) "
                      "VALUES ('roles-test', true) RETURNING 1"))
        self.assertAllowed(self.as_app('status', "DELETE FROM logger.health_checks "
                                                 "WHERE service = 'roles-test'"))
        self.assertRefused(self.as_app('status', 'CREATE TABLE logger.extra (x int)'))
        self.assertRefused(self.as_app('status', 'DROP TABLE logger.health_checks'),
                           'must be owner')
        self.assertRefused(self.as_app('auth', 'UPDATE oauth._prisma_migrations SET logs = NULL'))
        self.assertRefused(self.as_app('auth', 'CREATE TEMP TABLE t (x int)'))

    def test_no_app_role_is_a_superuser_or_runs_programs(self):
        for app in DATABASE_APPS:
            with self.subTest(app=app):
                result = self.as_app(app, f"SELECT rolsuper OR rolcreaterole OR rolcreatedb "
                                          f"FROM pg_roles WHERE rolname = '{app}_app'")
                self.assertEqual(result.stdout.strip(), 'f', result.stderr)
                self.assertRefused(self.as_app(app, "COPY (SELECT 1) TO PROGRAM 'id'"))

    def test_the_reader_sees_its_named_tables_and_nothing_else(self):
        for table in ('oauth.operators', 'oauth.apps', 'oauth.permissions', 'oauth.grants'):
            self.assertAllowed(self.as_reader(f'SELECT count(*) FROM {table}'))
        for table in ('oauth.guest_keys', 'calendar.calendar_scopes', 'sprinter.guards',
                      'logger.health_checks'):
            self.assertRefused(self.as_reader(f'SELECT count(*) FROM {table}'))
        self.assertRefused(self.as_reader('BEGIN READ WRITE; CREATE TEMP TABLE t (x int); COMMIT'))

    def test_rerunning_the_roles_is_idempotent_and_rotates_passwords(self):
        rotated = 'calendar' + uuid.uuid4().hex
        self.run_roles(CALENDAR_DB_PASSWORD=rotated)
        try:
            self.assertAllowed(self.sql('calendar_app', rotated, 'SELECT 1'))
            self.assertRefused(self.as_app('calendar', 'SELECT 1'), 'password authentication failed')
        finally:
            self.run_roles()
        self.assertAllowed(self.as_app('calendar', 'SELECT count(*) FROM calendar.calendar_scopes'))

    def test_a_migration_rerun_keeps_every_grant(self):
        self.migrate('auth')
        self.assertAllowed(self.as_reader('SELECT count(*) FROM oauth.grants'))
        self.assertAllowed(self.as_app('auth', 'SELECT count(*) FROM oauth.grants'))

    def test_migrate_refuses_a_bad_app_name(self):
        for app, message in (('../x', 'usage: migrate'), ('nope', 'has no prisma/schema.prisma')):
            result = self.migrate(app, check=False)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIn(message, result.stderr)


if __name__ == '__main__':
    unittest.main()
