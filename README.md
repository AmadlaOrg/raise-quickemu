# raise-quickemu

Raise plugin for managing VMs via quickemu/quickget.

Downloads official OS images directly from distribution mirrors (no intermediary registries) and manages QEMU virtual machines through quickemu's simple CLI.

## Prerequisites

- quickemu and quickget installed (`sudo apt install quickemu` or from https://github.com/quickemu-project/quickemu)
- QEMU installed

## Usage

```bash
raise up --provider quickemu -f vm.yaml myvm
raise halt --provider quickemu myvm
raise destroy --provider quickemu myvm
raise ssh --provider quickemu myvm
raise status --provider quickemu myvm
```

## Entity Format

```yaml
_type: amadla.org/entity/infrastructure/vm@v1.0.0
_body:
  os: ubuntu
  release: "22.04"
  edition: ""
  cpus: 2
  memory: "4G"
  disk_size: "32G"
  display: none
  ssh:
    user: user
    port: 22220
```

## Supported Operating Systems

Run `quickget` (with no arguments) to see all 300+ supported operating systems.

## How It Works

1. `raise-quickemu up` calls `quickget <os> <release>` to download the official ISO
2. Customizes the generated `.conf` file with CPU, RAM, disk settings
3. Launches the VM with `quickemu --vm <conf> --display none`
4. SSH is auto-forwarded to localhost:22220 by default

## License

MIT
