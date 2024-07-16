# syntax=docker/dockerfile:1.5
FROM ubuntu:22.04

ARG DEBIAN_FRONTEND=noninteractive
ARG COLLECTION_PATH=/usr/share/ansible/collections

RUN apt-get update && apt-get upgrade -y && \
    apt-get install -y --no-install-recommends \
        locales \
        openssh-client \
        software-properties-common \
        python3-pip \
        git && \
    localedef -i en_US -c -f UTF-8 -A /usr/share/locale/locale.alias en_US.UTF-8 && \
    apt-get clean && \
    rm -Rf /var/lib/apt/lists/* && \
    rm -Rf /usr/share/doc && rm -Rf /usr/share/man && \
    rm -rf /var/tmp* && rm -rf /tmp/*

ENV LANG en_US.UTF-8

RUN pip3 install --no-cache-dir ansible ansible-lint jmespath
# Add these python packages to above command for OpenTelemetry tracing for Ansible playbooks:
# opentelemetry-api opentelemetry-exporter-otlp opentelemetry-sdk

# These modules are included by default, but can add others:
# RUN mkdir -pm 755 ${COLLECTION_PATH} && \
#     ansible-galaxy collection install ansible.utils -p ${COLLECTION_PATH} && \
#     ansible-galaxy collection install community.general -p ${COLLECTION_PATH}

COPY ./out/ansible-tui-*-linux-amd64 /bin/ansible-tui

# Setup workdir and /etc/ansible/ansible.cfg
RUN mkdir -p /app/.ssh && chmod 750 /app && chmod 700 /app/.ssh && \
    mkdir /etc/ansible && \
    ansible-config init --disabled -t all > /etc/ansible/ansible.cfg && \
    sed -i 's/^;host_key_checking=True/host_key_checking=False/g' /etc/ansible/ansible.cfg

# Configure callbacks such as OpenTelemetry
#    sed -i 's/^;callbacks_enabled=.*$/callbacks_enabled = community.general.opentelemetry/' /etc/ansible/ansible.cfg && \
#    echo "" >> /etc/ansible/ansible.cfg && echo "[callback_opentelemetry]" >> /etc/ansible/ansible.cfg && \
#    echo "enable_from_environment = ANSIBLE_OPENTELEMETRY_ENABLED" >> /etc/ansible/ansible.cfg

WORKDIR /app

