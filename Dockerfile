# golang-builder is used in OSBS build
ARG GOLANG_BUILDER=golang:1.13
ARG OPERATOR_BASE_IMAGE=registry.access.redhat.com/ubi7/ubi-minimal:latest

FROM ${GOLANG_BUILDER} AS builder

ARG REMOTE_SOURCE=.
ARG REMOTE_SOURCE_DIR=neutron-operator
ARG REMOTE_SOURCE_SUBDIR=.
ARG DEST_ROOT=/dest-root
ARG GO_BUILD_EXTRA_ARGS="-v"

COPY $REMOTE_SOURCE $REMOTE_SOURCE_DIR
WORKDIR ${REMOTE_SOURCE_DIR}/${REMOTE_SOURCE_SUBDIR}

RUN mkdir -p ${DEST_ROOT}/usr/local/bin/

# Build
RUN CGO_ENABLED=0 GO111MODULE=on go build ${GO_BUILD_EXTRA_ARGS} -a -o ${DEST_ROOT}/usr/local/bin/manager main.go
RUN CGO_ENABLED=0 GO111MODULE=on go build ${GO_BUILD_EXTRA_ARGS} -a -o ${DEST_ROOT}/usr/local/bin/csv-generator tools/csv-generator.go

RUN cp tools/user_setup ${DEST_ROOT}/usr/local/bin/
RUN cp -r templates ${DEST_ROOT}/templates

# prep the bundle
RUN mkdir -p ${DEST_ROOT}/bundle
RUN cp config/crd/bases/neutron.openstack.org_neutronsriovagents.yaml ${DEST_ROOT}/bundle/neutron.openstack.org_neutronsriovagents.yaml
RUN cp config/crd/bases/neutron.openstack.org_ovncontrollers.yaml ${DEST_ROOT}/bundle/neutron.openstack.org_ovncontrollers.yaml
RUN cp config/crd/bases/neutron.openstack.org_ovsnodeosp.yaml ${DEST_ROOT}/bundle/neutron.openstack.org_ovsnodeosp.yaml

# strip top 2 lines (this resolves parsing in opm which handles this badly)
RUN sed -i -e 1,2d ${DEST_ROOT}/bundle/*

FROM ${OPERATOR_BASE_IMAGE}
ARG DEST_ROOT=/dest-root

LABEL   com.redhat.component="neutron-operator-container" \
        name="neutron-operator" \
        version="1.0" \
        summary="Neutron Operator" \
        io.k8s.name="neutron-operator" \
        io.k8s.description="This image includes the neutron-operator"

ENV USER_UID=1001 \
    OPERATOR_TEMPLATES=/usr/share/neutron-operator/templates/ \
    OPERATOR_BUNDLE=/usr/share/neutron-operator/bundle/

# install operator binary
COPY --from=builder ${DEST_ROOT}/usr/local/bin/* /usr/local/bin/

# install our templates
RUN  mkdir -p ${OPERATOR_TEMPLATES}
COPY --from=builder ${DEST_ROOT}/templates ${OPERATOR_TEMPLATES}

# install CRDs and required roles, services, etc
RUN  mkdir -p ${OPERATOR_BUNDLE}
COPY --from=builder ${DEST_ROOT}/bundle/* ${OPERATOR_BUNDLE}

WORKDIR /

# user setup
RUN  /usr/local/bin/user_setup
USER ${USER_UID}

ENTRYPOINT ["/usr/local/bin/manager"]
