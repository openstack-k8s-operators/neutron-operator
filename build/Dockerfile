FROM registry.access.redhat.com/ubi7/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/neutron-operator \
    USER_UID=1001 \
    USER_NAME=neutron-operator \
    OPERATOR_TEMPLATES=/usr/share/neutron-operator/templates/

# install operator binary
COPY build/_output/bin/neutron-operator ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

# install our templates
RUN  mkdir -p ${OPERATOR_TEMPLATES}
COPY templates ${OPERATOR_TEMPLATES}

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
