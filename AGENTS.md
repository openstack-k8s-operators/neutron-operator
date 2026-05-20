# AGENTS.md - neutron-operator

## Project overview

neutron-operator is a Kubernetes operator that manages
[OpenStack Neutron](https://docs.openstack.org/neutron/latest/) (the networking
service: virtual networks, routers, security groups, floating IPs, ML2/OVN,
DHCP, metadata) on OpenShift/Kubernetes. It is part of the
[openstack-k8s-operators](https://github.com/openstack-k8s-operators) project.

Key Neutron domain concepts: **virtual networks**, **subnets**, **routers**,
**security groups**, **floating IPs**, **ML2 plugin**, **OVN backend**,
**DHCP agent**, **metadata agent**, **SR-IOV agent**, **OVN agent**.

## Tech stack

| Layer | Technology |
|-------|------------|
| Language | Go (modules, multi-module workspace via `go.work`) |
| Scaffolding | [Kubebuilder v4](https://book.kubebuilder.io/) + [Operator SDK](https://sdk.operatorframework.io/) |
| CRD generation | controller-gen (DeepCopy, CRDs, RBAC, webhooks) |
| Config management | Kustomize |
| Packaging | OLM bundle |
| Testing | Ginkgo/Gomega + envtest (functional), KUTTL (integration) |
| Linting | golangci-lint (`.golangci.yaml`) |
| CI | Zuul (`zuul.d/`), Prow (`.ci-operator.yaml`), GitHub Actions |

## Custom Resources

| Kind | Purpose |
|------|---------|
| `NeutronAPI` | Manages the Neutron API deployment (httpd/WSGI). This is the only user-facing CR -- there is no top-level "Neutron" CR. |

The `NeutronAPI` CR has defaulting and validating admission webhooks.

## Directory structure

**Maintenance rule:** when directories are added, removed, or renamed, or when
their purpose changes, update this table to match.

| Directory | Contents |
|-----------|----------|
| `api/v1beta1/` | CRD types (`neutronapi_types.go`), conditions, webhook markers |
| `cmd/` | `main.go` entry point |
| `internal/controller/` | Reconciler: `neutronapi_controller.go` |
| `internal/neutronapi/` | NeutronAPI resource builders (deployment, volumes, config) |
| `internal/webhook/` | Webhook implementation |
| `templates/` | Config files via `OPERATOR_TEMPLATES` env var. The agent config files (`dhcp-agent.conf`, `ovn-agent.conf`, `ovn-metadata-agent.conf`, `sriov-agent.conf`) are used to create secrets consumed by dataplane services on EDPM nodes. `neutronapi/` contains config and httpd templates for the API deployment. |
| `config/crd,rbac,manager,webhook/` | Generated Kubernetes manifests (CRDs, RBAC, deployment, webhooks) |
| `config/samples/` | Example CRs (Kustomize overlays). Includes default, TLS, and Open vSwitch variants. |
| `test/functional/` | envtest-based Ginkgo/Gomega tests |
| `test/kuttl/` | KUTTL integration tests |
| `hack/` | Helper scripts (CRD schema checker, local webhook runner) |

## Build commands

After modifying Go code, always run: `make generate manifests fmt vet`.
Run `pre-commit run --all-files` before submitting to cover sanity, lint,
CRD schema checks, and bundle generation.

## Code style guidelines

- Follow standard [openstack-k8s-operators conventions](https://github.com/openstack-k8s-operators/dev-docs) and [lib-common](https://github.com/openstack-k8s-operators/lib-common) patterns.
- Use `lib-common` modules for conditions, endpoints, TLS, storage, and other
  cross-cutting concerns rather than re-implementing them.
- CRD types go in `api/v1beta1/`. Controller logic goes in
  `internal/controller/`. Resource-building helpers go in `internal/neutronapi/`.
- Config templates are plain files in `templates/` -- they are mounted at
  runtime via the `OPERATOR_TEMPLATES` environment variable.
- Webhook logic is split between the kubebuilder markers in `api/v1beta1/` and
  the implementation in `internal/webhook/`.

## Testing

- Functional tests use the envtest framework with Ginkgo/Gomega and live in
  `test/functional/`.
- KUTTL integration tests live in `test/kuttl/`.
- Run all functional tests: `make test`.
- When adding a new field or feature, add corresponding test cases in
  `test/functional/` and update fixture data accordingly.

## Key dependencies

- [lib-common](https://github.com/openstack-k8s-operators/lib-common): shared modules for conditions, endpoints, database, TLS, secrets, etc.
- [infra-operator](https://github.com/openstack-k8s-operators/infra-operator): RabbitMQ, Memcached, and topology APIs.
- [mariadb-operator](https://github.com/openstack-k8s-operators/mariadb-operator): database provisioning.
- [keystone-operator](https://github.com/openstack-k8s-operators/keystone-operator): identity service registration.
- [ovn-operator](https://github.com/openstack-k8s-operators/ovn-operator): OVN networking backend.
- [Developer guidelines](https://github.com/openstack-k8s-operators/dev-docs): cross-project development conventions and patterns. Read this first for guidelines that apply to all openstack-k8s-operators.
