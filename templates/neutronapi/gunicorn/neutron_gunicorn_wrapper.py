#!/usr/bin/env python3
"""
Wrapper script to run neutron with gunicorn.
This script works around the issue where neutron parses sys.argv during WSGI app initialization.
"""
import os
import sys
import ssl

# Set neutron configuration environment variables
os.environ['OS_NEUTRON_CONFIG_DIR'] = '/etc/neutron'
os.environ['OS_NEUTRON_CONFIG_FILES'] = 'neutron.conf;plugins/ml2/ml2_conf.ini'

# Clear sys.argv to prevent neutron from parsing gunicorn arguments
sys.argv = ['neutron-api']

# Now run gunicorn programmatically
if __name__ == '__main__':
    from gunicorn.app.base import BaseApplication
    import importlib

    class NeutronGunicornApp(BaseApplication):
        def __init__(self, app, options=None):
            self.options = options or {}
            self.application = app
            super().__init__()

        def load_config(self):
            for key, value in self.options.items():
                self.cfg.set(key.lower(), value)

        def load(self):
            return self.application

    # Import neutron WSGI application
    neutron_wsgi = importlib.import_module('neutron.wsgi.api')

    options = {
        'bind': '0.0.0.0:9696',
        'workers': 2,
        'worker_class': 'sync',
        'timeout': 60,
        'keepalive': 2,
        'max_requests': 5000,
        'max_requests_jitter': 50,
        'preload_app': True,
        'accesslog': '-',
        'errorlog': '-',
        'loglevel': 'info',
        'chdir': '/var/lib/neutron',
    }

    # Add improved TLS support if certificates are available
    import os.path
    internal_cert = '/etc/pki/tls/certs/internal.crt'
    internal_key = '/etc/pki/tls/private/internal.key'

    if os.path.exists(internal_cert) and os.path.exists(internal_key):
        options.update({
            'keyfile': internal_key,
            'certfile': internal_cert,
            # Use modern TLS protocol negotiation for better compatibility
            'ssl_version': ssl.PROTOCOL_TLS,
            'ciphers': 'ECDHE+AESGCM:ECDHE+CHACHA20:DHE+AESGCM:DHE+CHACHA20:!aNULL:!MD5:!DSS',
            'do_handshake_on_connect': True,
        })
        print(f"TLS enabled with enhanced configuration: cert={internal_cert}, key={internal_key}")
    else:
        print("TLS certificates not found, serving HTTP only")

    NeutronGunicornApp(neutron_wsgi.application, options).run()
