#!/bin/bash

DCOMMS_USE_PODMAN=1

sudo apt-get update

sudo apt-get install -y dnsutils curl aria2 tor 

if [ -z "$DCOMMS_USE_PODMAN" ]; then
    sudo apt-get install -y podman python3 python3-pip pipx firewalld

    pipx install podman-compose
    pipx ensurepath

    source ~/.bashrc

    # We redirect ports for rootless podman, because rootless podman can't bind ports 80 and 443
    sudo firewall-cmd --add-forward-port=port=80:proto=tcp:toport=8000 --permanent
    sudo firewall-cmd --add-forward-port=port=443:proto=tcp:toport=3443 --permanent
    sudo firewall-cmd --reload

    # FIXME: this path (/home/$USER/.config/containers/containers.conf) should work, but this doesn't affect podman pull for some reason 
    # mkdir -p /home/$USER/.config/containers/
    echo "unqualified-search-registries = [\"docker.io\"]" | sudo tee /etc/containers/registries.conf.d/10-unqualified-search-registries.conf

    systemctl --user enable podman.socket
else
    # The Docker path
    sudo apt-get update
    sudo apt-get install -y ca-certificates curl
    sudo install -m 0755 -d /etc/apt/keyrings
    sudo curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
    sudo chmod a+r /etc/apt/keyrings/docker.asc

    sudo tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/debian
Suites: $(. /etc/os-release && echo "$VERSION_CODENAME")
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF

    sudo apt-get update

    sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

    sudo usermod -aG docker $USER
fi

DIR=/home/$USER/dcomms

[ -d "$DIR" ] && sudo rm -r $DIR
mkdir -p $DIR

sudo mkdir -p /var/www/dcomms

sudo apt-get autoremove -y
