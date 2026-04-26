# netboot.xyz-style web VM

This example uses a Dockerfile as provisioning input. holos reads the
Dockerfile, pulls the `FROM` image, translates supported instructions into
cloud-init, copies `index.html` into the guest, and starts nginx.

```bash
holos up -f examples/netboot-xyz/holos.yaml
curl localhost:8080
holos down netboot-xyz
```

What it demonstrates:

- `dockerfile: ./Dockerfile`
- Dockerfile `RUN`, `COPY`, `ENV`, and `WORKDIR` translation
- Combining Dockerfile provisioning with extra `cloud_init.runcmd`
- Publishing guest port 80 as host port 8080

Dockerfile support is intentionally limited. Unsupported instructions fail
loudly instead of being skipped, so move runtime behavior into `cloud_init` or
guest systemd units.
