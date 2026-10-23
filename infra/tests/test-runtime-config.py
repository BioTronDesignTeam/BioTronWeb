#!/usr/bin/env python3
"""Exercise real Compose interpolation and launch selection, without a daemon."""
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
APPS = ('auth', 'calendar', 'exo', 'logger', 'site', 'sprinter')


class RuntimeConfigTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        paths = ['launch', 'docker-compose.yml', 'infra/docker-compose.yml',
                 'infra/edge/docker-compose.yml', 'infra/infisical/docker-compose.yml',
                 'infra/no-host-ports.yml']
        paths += [f'apps/{app}/docker-compose.yml' for app in APPS]
        for path in paths:
            dest = self.root / path
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(ROOT / path, dest)
        # Don't inherit user configuration or credentials into fixtures.
        self.env = {k: v for k, v in os.environ.items()
                    if k in ('PATH', 'HOME', 'DOCKER_CONFIG')}
        self.env.update(COMPOSE_DISABLE_ENV_FILE='true')

    def run_command(self, *args, success=True):
        result = subprocess.run(args, cwd=self.root, env=self.env,
                                capture_output=True, text=True)
        self.assertEqual(result.returncode == 0, success, result.stderr)
        return result

    def config(self, *args):
        return json.loads(self.run_command('docker', 'compose', *args,
                                          'config', '--format', 'json').stdout)

    def secrets(self):
        for app in APPS:
            if app != 'site':
                self.env[f'{app.upper()}_DATABASE_URL'] = f'fixture-{app}'
        self.env['LOGGER_INGEST_TOKEN'] = 'fixture-ingest'
        self.env.update(POSTGRES_PASSWORD='fixture-db', SPRINTER_READER_PASSWORD='fixture-reader')

    def test_no_env_files_required_to_parse_or_stop_web(self):
        self.assertEqual(len(self.config()['services']), 18)
        self.run_command('./launch', 'stop', 'web', '--runtime', '--dry-run')

    def test_database_and_public_config_are_isolated(self):
        self.secrets()
        self.env.update(AUTH_FRONTEND_URL='https://auth.example.test',
                        SPRINTER_FRONTEND_URL='https://sprinter.example.test',
                        AUTH_WEB_PORT='15173', SPRINTER_WEB_PORT='15178')
        services = self.config()['services']
        for app in APPS:
            if app == 'site':
                continue
            self.assertEqual(services[f'{app}-migrate']['environment']['DATABASE_URL'],
                             f'fixture-{app}')
        self.assertEqual(services['auth-api']['environment']['FRONTEND_URL'],
                         'https://auth.example.test')
        self.assertEqual(services['sprinter-bot']['environment']['FRONTEND_URL'],
                         'https://sprinter.example.test')
        self.assertEqual(services['auth-web']['ports'][0]['published'], '15173')
        self.assertEqual(services['sprinter-web']['ports'][0]['published'], '15178')

    def test_launch_accepts_injected_values(self):
        self.secrets()
        result = self.run_command('./launch', 'web', '--runtime', '--dry-run')
        self.assertIn('auth-migrate', result.stdout)
        self.assertNotIn('fixture-', result.stdout)

    def test_launch_fails_for_missing_selected_credentials(self):
        self.run_command('./launch', 'web', '--runtime', '--dry-run', success=False)
        self.secrets()
        del self.env['LOGGER_INGEST_TOKEN']
        self.run_command('./launch', 'web', '--runtime', '--dry-run', success=False)

    def test_excluded_apps_do_not_need_secrets(self):
        args = [f'--no-{app}' for app in APPS if app != 'site']
        result = self.run_command('./launch', 'web', '--runtime', '--dry-run', *args)
        self.assertIn('site-web', result.stdout)
        self.assertNotIn('auth-api', result.stdout)

    def test_local_files_work_and_runtime_ignores_them(self):
        (self.root / 'apps/auth/.env').write_text('DATABASE_URL=fixture-local\n')
        args = [f'--no-{app}' for app in APPS if app != 'auth']
        self.run_command('./launch', 'web', '--dry-run', *args)
        self.run_command('./launch', 'web', '--runtime', '--dry-run', *args,
                         success=False)
        self.env['AUTH_DATABASE_URL'] = 'fixture-injected'
        self.run_command('./launch', 'web', '--runtime', '--dry-run', *args)

    def test_edge_uses_environment_backed_tunnel_secret(self):
        self.env.update(EDGE_SITES='prod=example.test', CLOUDFLARE_TUNNEL_TOKEN='fixture-tunnel')
        config = self.config('-f', 'infra/edge/docker-compose.yml', '--env-file', '/dev/null')
        self.assertEqual(config['secrets']['cloudflare_tunnel_token']['environment'],
                         'CLOUDFLARE_TUNNEL_TOKEN')
        self.assertNotIn('CLOUDFLARE_TUNNEL_TOKEN',
                         config['services']['cloudflared'].get('environment', {}))

    def test_edge_is_its_own_project(self):
        self.env.update(POSTGRES_PASSWORD='fixture-db', SPRINTER_READER_PASSWORD='fixture-reader')
        infra = self.config('-f', 'infra/docker-compose.yml', '--env-file', '/dev/null')
        self.assertEqual(sorted(infra['services']), ['postgres', 'redis'])
        (self.root / 'edge.env').write_text('EDGE_SITES=local=localhost\n')
        self.env['EDGE_ENV_FILE'] = str(self.root / 'edge.env')
        result = self.run_command('./launch', 'edge', '--no-tunnel', '--dry-run')
        self.assertIn('infra/edge/docker-compose.yml', result.stdout)
        self.assertTrue(result.stdout.rstrip().endswith('up -d nginx'), result.stdout)
        self.run_command('./launch', 'infra', '--no-tunnel', '--dry-run', success=False)

    def test_edge_joins_each_instance_network_and_no_other(self):
        (self.root / 'edge.env').write_text(
            'EDGE_SITES=staging=staging.example.test prod=example.test\n')
        self.env['EDGE_ENV_FILE'] = str(self.root / 'edge.env')
        result = self.run_command('./launch', 'edge', '--dry-run')
        self.assertIn('docker network create biotron-staging', result.stdout)
        self.assertIn('docker network create biotron-prod', result.stdout)
        self.assertNotIn('create biotron\n', result.stdout)
        (self.root / 'edge.env').write_text('EDGE_SITES=Prod=example.test\n')
        self.run_command('./launch', 'edge', '--dry-run', success=False)

    def test_apps_carry_instance_qualified_aliases_for_the_edge(self):
        edge_targets = {'site-web', 'calendar-api', 'calendar-web', 'oauth-manager',
                        'oauth-web', 'exo-api', 'exo-web', 'logger-api', 'logger-web',
                        'sprinter', 'sprinter-web'}
        for instance in (None, 'staging'):
            if instance:
                self.env['BIOTRON_INSTANCE'] = instance
            prefix = instance or 'local'
            aliases = set()
            for service in self.config()['services'].values():
                for net in (service.get('networks') or {}).values():
                    aliases.update((net or {}).get('aliases') or [])
            self.assertTrue({f'{prefix}-{name}' for name in edge_targets} <= aliases)
        # Every upstream the edge names is one of those aliases.
        template = (ROOT / 'infra/nginx/sites/site.conf.template').read_text()
        upstreams = set(re.findall(r'http://\$\{INSTANCE\}-([a-z-]+):', template))
        self.assertEqual(upstreams, edge_targets)

    def test_every_server_of_an_instance_can_turn_crawlers_away(self):
        template = (ROOT / 'infra/nginx/sites/site.conf.template').read_text()
        servers = template.count('\nserver {')
        self.assertEqual(template.count('    ${ROBOTS}\n'), servers)
        nginx_conf = (ROOT / 'infra/nginx/nginx.conf').read_text()
        self.assertIn('add_header X-Robots-Tag $robots_tag always;', nginx_conf)
        snippet = (ROOT / 'infra/nginx/snippets/robots-noindex.conf').read_text()
        self.assertIn('Disallow: /', snippet)

    def test_local_project_keeps_its_names(self):
        config = self.config()
        self.assertEqual(config['name'], 'biotron')
        self.assertEqual(config['networks']['biotron']['name'], 'biotron')
        self.assertEqual(config['volumes']['postgres-data']['name'],
                         'biotron-infrastructure_postgres-data')

    def test_instances_get_their_own_project_network_and_volumes(self):
        self.env.update(BIOTRON_INSTANCE='staging', BIOTRON_NETWORK='biotron-staging')
        config = self.config('-p', 'biotron-staging')
        self.assertEqual(config['name'], 'biotron-staging')
        self.assertEqual({net['name'] for net in config['networks'].values()},
                         {'biotron-staging'})
        self.assertEqual({vol['name'] for vol in config['volumes'].values()},
                         {'biotron-staging_postgres-data', 'biotron-staging_redis-data'})
        for service in config['services'].values():
            self.assertNotIn('container_name', service)

    def test_launch_runs_an_instance_as_its_own_project(self):
        self.secrets()
        result = self.run_command('./launch', 'all', '--instance', 'prod', '--runtime', '--dry-run')
        self.assertIn('docker network create biotron-prod', result.stdout)
        ups = [line for line in result.stdout.splitlines() if ' up ' in line]
        self.assertEqual(len(ups), 2, result.stdout)
        self.assertTrue(all('-p biotron-prod' in line for line in ups), result.stdout)
        self.assertTrue(ups[0].endswith('up -d postgres redis'), ups[0])
        self.assertNotIn('postgres', ups[1])

    def test_named_instances_publish_no_host_ports(self):
        local = self.config()['services']
        self.assertTrue(any(service.get('ports') for service in local.values()))
        self.env.update(BIOTRON_INSTANCE='staging', BIOTRON_NETWORK='biotron-staging')
        config = self.config('-p', 'biotron-staging', '-f', 'docker-compose.yml',
                             '-f', 'infra/no-host-ports.yml')
        self.assertEqual(sorted(config['services']), sorted(local))
        for name, service in config['services'].items():
            self.assertFalse(service.get('ports'), name)
        self.secrets()
        result = self.run_command('./launch', 'web', '--instance', 'staging', '--dry-run')
        self.assertIn('-f infra/no-host-ports.yml', result.stdout)

    def test_pull_deploys_tagged_images_without_building(self):
        self.secrets()
        args = ('./launch', 'all', '--instance', 'staging', '--runtime', '--pull', '--dry-run')
        self.run_command(*args, success=False)
        self.env.update(IMAGE_REGISTRY='ghcr.io/biotrondesignteam/', IMAGE_TAG='0' * 40)
        lines = self.run_command(*args).stdout.splitlines()
        pulls = [line for line in lines if ' pull ' in line]
        ups = [line for line in lines if ' up ' in line]
        self.assertEqual(len(pulls), 2)
        self.assertEqual(len(ups), 2)
        for line in ups:
            self.assertIn('--no-build', line)
            self.assertIn('--remove-orphans', line)
            self.assertNotIn('--build ', line)
        self.assertLess(lines.index(pulls[0]), lines.index(ups[0]))
        self.run_command('./launch', 'web', '--pull', '--build', '--dry-run', success=False)

    def test_launch_rejects_bad_or_shared_instance_names(self):
        for name in ('Prod', 'edge', 'secrets', 'a-b', ''):
            self.run_command('./launch', 'web', '--instance', name, '--dry-run', success=False)
        self.run_command('./launch', 'edge', '--instance', 'prod', '--dry-run', success=False)

    def test_infra_needs_database_passwords(self):
        self.run_command('./launch', 'infra', '--runtime', '--dry-run', success=False)
        self.env.update(POSTGRES_PASSWORD='fixture-db', SPRINTER_READER_PASSWORD='fixture-reader')
        self.run_command('./launch', 'infra', '--runtime', '--dry-run')

    def test_infisical_is_independent_and_loopback_only(self):
        self.env.update(ENCRYPTION_KEY='fixture-key', AUTH_SECRET='fixture-auth',
                        INFISICAL_DB_PASSWORD='fixture-db')
        config = self.config('-f', 'infra/infisical/docker-compose.yml',
                             '--env-file', '/dev/null')
        self.assertEqual(config['name'], 'biotron-secrets')
        self.assertNotIn('biotron', config['networks'])
        self.assertEqual(config['services']['infisical']['ports'][0]['host_ip'],
                         '127.0.0.1')
        self.assertNotIn('ports', config['services']['db'])
        self.assertNotIn('ports', config['services']['redis'])
        self.assertEqual(config['volumes']['postgres-data']['name'],
                         'biotron-secrets_postgres-data')
        self.assertNotIn('infisical', self.config()['services'])
        (self.root / 'bootstrap.env').write_text(
            'ENCRYPTION_KEY=fixture-key\nAUTH_SECRET=fixture-auth\n'
            'INFISICAL_DB_PASSWORD=fixture-db\n')
        self.env['INFISICAL_BOOTSTRAP_FILE'] = str(self.root / 'bootstrap.env')
        result = self.run_command('./launch', 'secrets', '--dry-run')
        self.assertIn('infra/infisical/docker-compose.yml', result.stdout)
        self.run_command('./launch', 'stop', 'secrets', '--dry-run')


if __name__ == '__main__':
    unittest.main()
