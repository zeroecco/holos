# GPU passthrough

This is a template for VFIO/PCI passthrough. It is not copy-paste runnable until
you replace the PCI addresses with devices from your host and complete IOMMU
setup.

```bash
holos devices --gpu
holos validate -f examples/gpu-passthrough/holos.yaml
```

What it demonstrates:

- PCI device declarations with `devices`
- Automatic UEFI enablement when devices are present
- Larger CPU and memory settings for accelerator workloads
- A port forward for notebooks or model-serving processes

Before running it, enable IOMMU in firmware and the kernel, bind the target GPU
and its audio function to `vfio-pci`, and make sure both functions are in a safe
IOMMU group.
