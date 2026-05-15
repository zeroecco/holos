# Simple inference

This example is a GPU-passthrough DeepSeek inference template. It starts Ollama
in an Ubuntu VM, pulls `deepseek-r1:7b`, publishes Ollama's API, and stores model
files on a named volume.

It is not copy-paste runnable until you replace the PCI addresses in
`holos.yaml`, complete host IOMMU/VFIO setup, and install the right guest GPU
driver. First boot also downloads Ollama and the model, so it can take several
minutes.

```bash
holos devices --gpu
holos up -f examples/simple-inference/holos.yaml
curl http://localhost:11434/api/tags
curl http://localhost:11434/api/generate \
  -H 'Content-Type: application/json' \
  -d '{"model":"deepseek-r1:7b","prompt":"Explain KVM in one sentence.","stream":false}'
holos down simple-inference
```

What it demonstrates:

- GPU passthrough for an inference-style VM
- Automatic UEFI enablement when PCI devices are present
- Ollama serving a useful local DeepSeek model
- Publishing guest port 11434 as host port 11434
- Persisting model files on a named volume
- A healthcheck that waits for the model to be available
