// Package vfio discovers PCI devices and IOMMU groups from sysfs for
// VFIO passthrough workflows.
//
// [ListIOMMUGroups] reads /sys/kernel/iommu_groups to enumerate all groups
// and their member devices, including PCI address, class, vendor/device IDs,
// and bound driver.
//
// [ListGPUs] filters the device list to VGA controllers (class 0300) and
// 3D controllers (class 0302), which is the common case for GPU passthrough.
package vfio
