package validation

import (
	"fmt"
	"strings"

	"github.com/IBM-Cloud/bluemix-go/crn"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openshift/installer/pkg/types/ibmcloud"
)

// ValidateMachinePool validates the MachinePool.
func ValidateMachinePool(platform *ibmcloud.Platform, mp *ibmcloud.MachinePool, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, zone := range mp.Zones {
		if !strings.HasPrefix(zone, platform.Region) {
			allErrs = append(allErrs, field.Invalid(path.Child("zones").Index(i), zone, fmt.Sprintf("zone not in configured region (%s)", platform.Region)))
		}
	}

	if mp.DedicatedHosts != nil {
		allErrs = append(allErrs, validateDedicatedHosts(mp.DedicatedHosts, mp.InstanceType, mp.Zones, path.Child("dedicatedHosts"))...)

		if mp.InstanceType == "" {
			allErrs = append(allErrs, field.Invalid(path.Child("type"), mp.InstanceType, "type is required, default type not supported for dedicated hosts"))
		}
	}

	if mp.BootVolume != nil {
		allErrs = append(allErrs, validateBootVolume(mp.BootVolume, path.Child("bootVolume"))...)
	}
	return allErrs
}

// Boot volume profiles supported by IBM Cloud VPC. The first generation
// profiles share a common set of limits, while sdp is the second generation
// profile with independently adjustable IOPS and bandwidth.
const (
	customProfile = "custom"
	sdpProfile    = "sdp"

	// Boot volume size limits, in GiB.
	minBootVolumeSizeGiB    = 10
	maxBootVolumeSizeGiB    = 250
	maxSDPBootVolumeSizeGiB = 32000
)

var validBootVolumeProfiles = []string{"general-purpose", "5iops-tier", "10iops-tier", customProfile, sdpProfile}

func validateBootVolume(bv *ibmcloud.BootVolume, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if bv.EncryptionKey != "" {
		_, parseErr := crn.Parse(bv.EncryptionKey)
		if parseErr != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("encryptionKey"), bv.EncryptionKey, "encryptionKey is not a valid IBM CRN"))
		}
	}

	if bv.Profile != "" && !sets.NewString(validBootVolumeProfiles...).Has(bv.Profile) {
		allErrs = append(allErrs, field.NotSupported(path.Child("profile"), bv.Profile, validBootVolumeProfiles))
	}

	// The second generation sdp profile supports capacities well beyond the
	// first generation limit.
	maxSizeGiB := int64(maxBootVolumeSizeGiB)
	if bv.Profile == sdpProfile {
		maxSizeGiB = maxSDPBootVolumeSizeGiB
	}
	if bv.SizeGiB != 0 && (bv.SizeGiB < minBootVolumeSizeGiB || bv.SizeGiB > maxSizeGiB) {
		allErrs = append(allErrs, field.Invalid(path.Child("sizeGiB"), bv.SizeGiB, fmt.Sprintf("size must be between %d and %d GiB for the %s profile", minBootVolumeSizeGiB, maxSizeGiB, bootVolumeProfileOrDefault(bv.Profile))))
	}

	// IOPS is only user configurable on the custom and sdp profiles; the other
	// profiles derive it from the profile itself.
	if bv.IOPS != 0 && bv.Profile != customProfile && bv.Profile != sdpProfile {
		allErrs = append(allErrs, field.Invalid(path.Child("iops"), bv.IOPS, fmt.Sprintf("iops is only supported for the %s and %s profiles", customProfile, sdpProfile)))
	}

	// Bandwidth is exclusive to the second generation sdp profile.
	if bv.Bandwidth != 0 && bv.Profile != sdpProfile {
		allErrs = append(allErrs, field.Invalid(path.Child("bandwidth"), bv.Bandwidth, fmt.Sprintf("bandwidth is only supported for the %s profile", sdpProfile)))
	}

	return allErrs
}

// bootVolumeProfileOrDefault returns the profile used for error messages,
// accounting for the profile being defaulted when unset.
func bootVolumeProfileOrDefault(profile string) string {
	if profile == "" {
		return "general-purpose"
	}
	return profile
}

func validateDedicatedHosts(dhosts []ibmcloud.DedicatedHost, itype string, zones []string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Length of dedicated hosts must match platform zones
	if len(dhosts) != len(zones) {
		allErrs = append(allErrs, field.Invalid(path, dhosts, fmt.Sprintf("number of dedicated hosts does not match list of zones (%s)", zones)))
	}

	for i, dhost := range dhosts {
		// Dedicated host name or profile is required
		if dhost.Name == "" && dhost.Profile == "" {
			allErrs = append(allErrs, field.Invalid(path.Index(i), dhost.Profile, "name or profile must be set"))
		}

		// Instance type must be in the same profile family as dedicated host
		if dhost.Profile != "" && itype != "" {
			if strings.Split(dhost.Profile, "-")[0] != strings.Split(itype, "-")[0] {
				allErrs = append(allErrs, field.Invalid(path.Index(i).Child("profile"), dhost.Profile, fmt.Sprintf("profile does not support expected instance type (%s)", itype)))
			}
		}
	}

	return allErrs
}
