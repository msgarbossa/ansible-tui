FROM ubuntu:22.04

ARG DEBIAN_FRONTEND=noninteractive
ARG COLLECTION_PATH=/usr/share/ansible/collections

RUN apt-get update && apt-get upgrade -y && \
    apt-get install -y --no-install-recommends \
        openssh-client \
        software-properties-common \
        python3-pip \
        git && \
    apt-get clean && \
    rm -Rf /var/lib/apt/lists/* && \
    rm -Rf /usr/share/doc && rm -Rf /usr/share/man && \
    rm -rf /var/tmp* && rm -rf /tmp/*

RUN pip3 install ansible

# These modules are included by default, but can add others:
# RUN mkdir -pm 755 ${COLLECTION_PATH} && \
#     ansible-galaxy collection install ansible.utils -p ${COLLECTION_PATH} && \
#     ansible-galaxy collection install community.general -p ${COLLECTION_PATH}

COPY ./out/ansible-shim-*-linux-amd64 /bin/ansible-shim

RUN mkdir -p /app/.ssh && chmod 750 /app && chmod 700 /app/.ssh && \
    mkdir /etc/ansible && \
    ansible-config init --disabled -t all > /etc/ansible/ansible.cfg && \
    sed -i 's/^;host_key_checking=True/host_key_checking=False/g' /etc/ansible/ansible.cfg

WORKDIR /app

ENTRYPOINT ["/bin/ansible-shim"]
