# Gunicorn configuration for Neutron API
import multiprocessing
import os

# Server socket
bind = "0.0.0.0:9696"

# Worker processes
workers = 2
worker_class = "sync"
worker_connections = 1000

# Process naming
proc_name = "neutron-api"

# Worker timeout and lifecycle
timeout = 60
keepalive = 2
max_requests = 5000
max_requests_jitter = 50

# Application preloading
preload_app = True

# Logging
accesslog = "-"
errorlog = "-"
loglevel = "info"
access_log_format = '%(h)s %(l)s %(u)s %(t)s "%(r)s" %(s)s %(b)s "%(f)s" "%(a)s"'

# Security limits
limit_request_line = 4094
limit_request_fields = 100
limit_request_field_size = 8190

# Working directory
chdir = "/var/lib/neutron"

# SSL/TLS configuration (when certificates are available)
# Uncomment and configure when TLS is enabled:
# keyfile = "/etc/pki/tls/private/internal.key"
# certfile = "/etc/pki/tls/certs/internal.crt"
# ssl_version = ssl.PROTOCOL_TLSv1_2
# ciphers = "ECDHE+AESGCM:ECDHE+CHACHA20:DHE+AESGCM:DHE+CHACHA20:!aNULL:!MD5:!DSS"
