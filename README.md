# homelab

This repository contains the Ansible playbooks and configurations I use to automatically provision and manage my local infrastructure. I mostly use this setup as a sandbox for system design, C programming, and coursework (like my CS6200 dev environment), but it also runs my daily network services.

## the setup

Everything is orchestrated with Ansible and tied together securely using Tailscale MagicDNS. 

* **Hypervisor:** Proxmox VE running on an Optiplex 3020.
* **DNS & Adblocking:** AdGuard Home running on a Raspberry Pi.
* **Monitoring:** Prometheus and Grafana (with a custom matrix-themed dashboard).
* **Dev Environments:** Assorted LXCs and VMs for development and testing.

Some of the more useful automations in here:
* Uses the Proxmox dynamic inventory plugin to grab LXC/VM IPs on the fly so I don't have to hardcode them in my hosts file.
* Automatically syncs new Proxmox containers to AdGuard Home as DNS rewrites.
* Handles baseline security across all nodes (disabling root login, setting up passwordless sudo, and pulling my GitHub SSH keys).

## how to use it

If you want to adapt this for your own lab, you'll need to set up your own inventory and secrets. 

### 1. set up your inventory
Copy the example files and plug in your own Tailscale IPs or domains:
```bash
cp ansible/inventory/hosts.example ansible/inventory/hosts
cp ansible/inventory/inventory.proxmox.example.yml ansible/inventory/inventory.proxmox.yml

```

### 2. proxmox credentials

To keep secrets out of the codebase, the dynamic inventory expects environment variables. Export these before running anything (or throw them in a local `.env` file):

```bash
export PROXMOX_URL="https://your-proxmox-ip:8006/"
export PROXMOX_TOKEN_ID="your-user@pam!your-token-name"
export PROXMOX_TOKEN_SECRET="your-secret-uuid"

```

### 3. ansible vault

Passwords and user hashes are encrypted using Ansible Vault.

```bash
# Create your local password file (this is already gitignored)
echo "your-vault-password" > ansible/.vault_password

# Copy the template and encrypt it
cp ansible/group_vars/all/vault.example.yml ansible/group_vars/all/vault.yml
ansible-vault encrypt ansible/group_vars/all/vault.yml

```

### 4. deploy

Run playbooks from inside the `ansible/` directory. For example, to stand up the monitoring stack:

```bash
ansible-playbook playbooks/setup-monitoring-server.yml

```

**Note:** Double-check that your `.env` and `vault.yml` files are actually being ignored by your git setup before committing anything.

```
