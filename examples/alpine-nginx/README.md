# Alpine nginx

This is the smallest runnable web example. It starts two Alpine VMs, installs
nginx through cloud-init, starts the OpenRC service, and publishes guest port 80
on host ports 8080 and 8081.

```bash
holos up -f examples/alpine-nginx/holos.yaml
curl localhost:8080
curl localhost:8081
holos down alpine-nginx
```

What it demonstrates:

- Image aliases (`image: alpine`)
- Replicas
- Static host port auto-increment across replicas
- Alpine/OpenRC first-boot setup with `cloud_init.runcmd`
