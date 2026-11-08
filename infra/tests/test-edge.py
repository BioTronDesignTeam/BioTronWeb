#!/usr/bin/env python3
"""Build the edge image and check what 40-biotron-sites.sh renders and what
the running Nginx answers. Needs a Docker daemon; the image and containers it
makes are removed at the end.

    python3 infra/tests/test-edge.py
"""
from pathlib import Path
import subprocess
import time
import unittest
import uuid

ROOT = Path(__file__).resolve().parents[2]
SITES = 'staging=uwbiotron.dev:noindex prod=uwbiotron.ca'


def docker(*args, check=True):
    return subprocess.run(['docker', *args], capture_output=True, text=True, check=check)


def docker_available():
    try:
        return docker('info', check=False).returncode == 0
    except FileNotFoundError:
        return False


@unittest.skipUnless(docker_available(), 'needs a Docker daemon')
class EdgeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.name = 'biotron-edge-test-' + uuid.uuid4().hex[:8]
        cls.image = cls.name
        docker('build', '-q', '-t', cls.image, str(ROOT / 'infra/nginx'))
        cls.containers = []

    @classmethod
    def tearDownClass(cls):
        for container in cls.containers:
            docker('rm', '-f', container, check=False)
        docker('image', 'rm', '-f', cls.image, check=False)

    def render(self, sites, subnet='10.213.255.0/29'):
        """Runs only the sites script, then nginx -t, and prints the maps."""
        env = ['-e', f'EDGE_SITES={sites}'] if sites is not None else []
        if subnet is not None:
            env += ['-e', f'EDGE_SUBNET={subnet}']
        return docker('run', '--rm', *env, '--entrypoint', 'sh', self.image, '-c',
                      '/docker-entrypoint.d/40-biotron-sites.sh && nginx -t -q && '
                      'ls /etc/nginx/conf.d && cat /etc/nginx/conf.d/01-maps.conf '
                      '/etc/nginx/conf.d/02-real-ip.conf', check=False)

    def start(self, subnet='10.213.255.0/29'):
        container = f'{self.name}-{len(self.containers)}'
        self.containers.append(container)
        docker('run', '-d', '--name', container, '-e', f'EDGE_SITES={SITES}',
               '-e', f'EDGE_SUBNET={subnet}', '--read-only', '--tmpfs', '/etc/nginx/conf.d',
               '--tmpfs', '/tmp', self.image)
        for _ in range(50):
            if self.get(container, 'uwbiotron.dev', '/_edge/health').returncode == 0:
                return container
            time.sleep(0.2)
        raise AssertionError(docker('logs', container, check=False).stderr)

    def get(self, container, host, path, *headers, post=None):
        """wget inside the container; prints the response headers to stderr."""
        args = ['wget', '-qSO-', '--header', f'Host: {host}']
        for header in headers:
            args += ['--header', header]
        if post is not None:
            args += ['--header', 'Content-Type: application/csp-report', '--post-data', post]
        return docker('exec', container, *args, f'http://127.0.0.1:8080{path}', check=False)

    def test_renders_each_instance_and_its_flags(self):
        result = self.render(SITES)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('site-staging.conf', result.stdout)
        self.assertIn('site-prod.conf', result.stdout)
        self.assertIn('.uwbiotron.dev "noindex, nofollow";', result.stdout)
        self.assertNotIn('.uwbiotron.ca "noindex', result.stdout)
        self.assertIn('https://auth.uwbiotron.ca', result.stdout)
        self.assertIn("'sha256-", result.stdout)
        self.assertIn('set_real_ip_from 10.213.255.0/29;', result.stdout)

    def test_refuses_bad_sites_and_subnets(self):
        for sites in ('Prod=x.ca', 'prod=x.ca:foo', 'prod=x.ca prod=y.ca', 'a=x.ca b=x.ca',
                      'prod=x.ca;evil', 'prod', '', None):
            with self.subTest(sites=sites):
                result = self.render(sites)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn('EDGE_SITES', result.stderr)
        for subnet in ('10.213.255.0', '10.213.255.0/29; deny all', None):
            with self.subTest(subnet=subnet):
                result = self.render(SITES, subnet)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn('EDGE_SUBNET', result.stderr)

    def test_security_headers(self):
        container = self.start()
        result = self.get(container, 'calendar.uwbiotron.ca', '/_edge/health',
                          'X-Forwarded-Proto: https')
        self.assertIn('Strict-Transport-Security: max-age=300\n', result.stderr)
        self.assertNotIn('includeSubDomains', result.stderr)
        self.assertIn("Content-Security-Policy: default-src 'self'; script-src 'self' 'sha256-",
                      result.stderr)
        self.assertIn('Reporting-Endpoints: csp="/_edge/csp-report"', result.stderr)
        self.assertNotIn('X-Robots-Tag', result.stderr)
        # Plain HTTP, as from a local curl, gets no HSTS.
        result = self.get(container, 'calendar.uwbiotron.ca', '/_edge/health')
        self.assertNotIn('Strict-Transport-Security', result.stderr)
        # Staging is noindex.
        result = self.get(container, 'uwbiotron.dev', '/robots.txt')
        self.assertIn('X-Robots-Tag: noindex, nofollow', result.stderr)
        self.assertIn('Disallow: /', result.stdout)

    def test_redirects_stay_relative(self):
        container = self.start()
        result = self.get(container, 'calendar.uwbiotron.ca', '/api')
        self.assertIn('Location: /api/\n', result.stderr)
        self.assertNotIn(':8080', result.stderr)

    def test_logs_csp_reports_without_query_strings(self):
        container = self.start()
        report = ('{"csp-report":{"document-uri":"https://auth.uwbiotron.ca/cb?code=SECRET",'
                  '"effective-directive":"script-src-elem",'
                  '"blocked-uri":"https://evil.example/x.js?SECRET","disposition":"enforce"}}')
        result = self.get(container, 'auth.uwbiotron.ca', '/_edge/csp-report', post=report)
        self.assertIn('204 No Content', result.stderr)
        # Only a POST is a report; the sink answers only the edge itself.
        self.assertNotEqual(self.get(container, 'auth.uwbiotron.ca', '/_edge/csp-report').returncode, 0)
        self.assertNotEqual(self.get(container, 'auth.uwbiotron.ca', '/_edge/csp-sink').returncode, 0)
        logs = docker('logs', container).stdout
        line = next(l for l in logs.splitlines() if '"type":"csp-report"' in l and 'script-src' in l)
        self.assertIn('"directive":"script-src-elem"', line)
        self.assertIn('"blocked":"https://evil.example/x.js"', line)
        self.assertIn('"document":"https://auth.uwbiotron.ca/cb"', line)
        self.assertNotIn('SECRET', logs)

    def test_believes_cf_connecting_ip_only_from_the_edge_network(self):
        # 127.0.0.1 is outside the edge network, so its header is ignored.
        container = self.start()
        self.get(container, 'www.uwbiotron.ca', '/x', 'Cf-Connecting-Ip: 6.6.6.6')
        self.assertNotIn('6.6.6.6', docker('logs', container).stdout)
        # From inside the trusted range, the header names the client.
        container = self.start(subnet='127.0.0.0/8')
        self.get(container, 'www.uwbiotron.ca', '/x', 'Cf-Connecting-Ip: 6.6.6.6')
        self.assertIn('"client_ip":"6.6.6.6"', docker('logs', container).stdout)


if __name__ == '__main__':
    unittest.main()
