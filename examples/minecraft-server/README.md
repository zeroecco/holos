# Minecraft server

This example starts a vanilla Minecraft Java server in an Ubuntu VM. On first
boot it installs Java, downloads the latest official server jar from Mojang,
accepts the Minecraft EULA, and publishes guest port 25565 on host port 25565.

```bash
holos up -f examples/minecraft-server/holos.yaml
holos logs minecraft-server
holos down minecraft-server
```

Connect your Minecraft client to `localhost:25565` once the `minecraft` systemd
service is healthy.

What it demonstrates:

- A game server with a larger VM profile
- A named volume for persistent world data
- A first-boot setup script written with `cloud_init.write_files`
- A systemd service managed from `cloud_init.runcmd`
- A healthcheck for service readiness

The `world` volume survives `holos down`. Remove the project volume from your
holos state directory if you want to delete the world.
