package spdx22json

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/anchore/syft/internal"
	"github.com/anchore/syft/internal/log"
	"github.com/anchore/syft/internal/spdxlicense"
	"github.com/anchore/syft/syft/artifact"
	"github.com/anchore/syft/syft/file"
	"github.com/anchore/syft/syft/formats/common/spdxhelpers"
	"github.com/anchore/syft/syft/formats/spdx22json/model"
	"github.com/anchore/syft/syft/pkg"
	"github.com/anchore/syft/syft/rekor"
	"github.com/anchore/syft/syft/sbom"
	"github.com/anchore/syft/syft/source"
)

// toFormatModel creates and populates a new JSON document struct that follows the SPDX 2.2 spec from the given cataloging results.
func toFormatModel(s sbom.SBOM) *model.Document {
	name, namespace := spdxhelpers.DocumentNameAndNamespace(s.Source)

	relationships := s.RelationshipsSorted()

	return &model.Document{
		Element: model.Element{
			SPDXID: model.ElementID("DOCUMENT").String(),
			Name:   name,
		},
		SPDXVersion: model.Version,
		CreationInfo: model.CreationInfo{
			Created: time.Now().UTC(),
			Creators: []string{
				// note: key-value format derived from the JSON example document examples: https://github.com/spdx/spdx-spec/blob/v2.2/examples/SPDXJSONExample-v2.2.spdx.json
				"Organization: Anchore, Inc",
				"Tool: " + internal.ApplicationName + "-" + s.Descriptor.Version,
			},
			LicenseListVersion: spdxlicense.Version,
		},
<<<<<<< HEAD
		DataLicense:          "CC0-1.0",
		ExternalDocumentRefs: toExternalDocumentRefs(s.Relationships),
		DocumentNamespace:    namespace,
		Packages:             toPackages(s.Artifacts.PackageCatalog, s.Relationships),
		Files:                toFiles(s),
		Relationships:        toRelationships(s.Relationships),
=======
		DataLicense:       "CC0-1.0",
		DocumentNamespace: namespace,
		Packages:          toPackages(s.Artifacts.PackageCatalog, relationships),
		Files:             toFiles(s),
		Relationships:     toRelationships(relationships),
>>>>>>> c2005fa (Stabilize SPDX JSON output sorting (#1216))
	}
}

// isValidExternalRelationshipDocument returns if rel contains an ExternalRef and if it to_format_model know how to handle it.
// An error is returned if rel contains an ExternalRef, but the rel cannot be handled
func isValidExternalRelationshipDocument(rel artifact.Relationship) (bool, error) {
	if _, ok := rel.From.(rekor.ExternalRef); ok {
		return false, errors.New("syft cannot handle an ExternalRef in the FROM field of a relationship")
	}
	if externalRef, ok := rel.To.(rekor.ExternalRef); ok {
		relationshipType := artifact.DescribedByRelationship
		if rel.Type == relationshipType && toChecksumAlgorithm(externalRef.SpdxRef.Alg) == "SHA1" { // spdx 2.2 spec requires an sha1 hash
			return true, nil
		}
		return false, fmt.Errorf("syft cannot handle an ExternalRef with relationship type: %v", relationshipType)
	}
	return false, nil
}

func toExternalDocumentRefs(relationships []artifact.Relationship) []model.ExternalDocumentRef {
	externalDocRefs := []model.ExternalDocumentRef{}
	for _, rel := range relationships {
		valid, err := isValidExternalRelationshipDocument(rel)
		if err != nil {
			log.Warnf("dropping relationship %v: %w", rel, err)
			continue
		}
		if valid {
			externalRef := rel.To.(rekor.ExternalRef)
			externalDocRef := model.ExternalDocumentRef{
				ExternalDocumentID: model.DocElementID(rel.To.ID()).String(),
				Checksum: model.Checksum{
					Algorithm:     toChecksumAlgorithm(externalRef.SpdxRef.Alg),
					ChecksumValue: externalRef.SpdxRef.Checksum,
				},
				SpdxDocument: externalRef.SpdxRef.URI,
			}
			externalDocRefs = append(externalDocRefs, externalDocRef)
		}
	}
	return externalDocRefs
}

func toPackages(catalog *pkg.Catalog, relationships []artifact.Relationship) []model.Package {
	packages := make([]model.Package, 0)

	for _, p := range catalog.Sorted() {
		license := spdxhelpers.License(p)
		packageSpdxID := model.ElementID(p.ID()).String()
		filesAnalyzed := false

		// we generate digest for some Java packages
		// see page 33 of the spdx specification for 2.2
		// spdx.github.io/spdx-spec/package-information/#710-package-checksum-field
		var checksums []model.Checksum
		if p.MetadataType == pkg.JavaMetadataType {
			javaMetadata := p.Metadata.(pkg.JavaMetadata)
			if len(javaMetadata.ArchiveDigests) > 0 {
				filesAnalyzed = true
				for _, digest := range javaMetadata.ArchiveDigests {
					checksums = append(checksums, model.Checksum{
						Algorithm:     strings.ToUpper(digest.Algorithm),
						ChecksumValue: digest.Value,
					})
				}
			}
		}
		// note: the license concluded and declared should be the same since we are collecting license information
		// from the project data itself (the installed package files).
		packages = append(packages, model.Package{
			Checksums:        checksums,
			Description:      spdxhelpers.Description(p),
			DownloadLocation: spdxhelpers.DownloadLocation(p),
			ExternalRefs:     spdxhelpers.ExternalRefs(p),
			FilesAnalyzed:    filesAnalyzed,
			HasFiles:         fileIDsForPackage(packageSpdxID, relationships),
			Homepage:         spdxhelpers.Homepage(p),
			// The Declared License is what the authors of a project believe govern the package
			LicenseDeclared: license,
			Originator:      spdxhelpers.Originator(p),
			SourceInfo:      spdxhelpers.SourceInfo(p),
			VersionInfo:     p.Version,
			Item: model.Item{
				// The Concluded License field is the license the SPDX file creator believes governs the package
				LicenseConcluded: license,
				Element: model.Element{
					SPDXID: packageSpdxID,
					Name:   p.Name,
				},
			},
		})
	}

	return packages
}

func fileIDsForPackage(packageSpdxID string, relationships []artifact.Relationship) (fileIDs []string) {
	for _, relationship := range relationships {
		if relationship.Type != artifact.ContainsRelationship {
			continue
		}

		if _, ok := relationship.From.(pkg.Package); !ok {
			continue
		}

		if _, ok := relationship.To.(source.Coordinates); !ok {
			continue
		}

		from := model.ElementID(relationship.From.ID()).String()
		if from == packageSpdxID {
			to := model.ElementID(relationship.To.ID()).String()
			fileIDs = append(fileIDs, to)
		}
	}
	return fileIDs
}

func toFiles(s sbom.SBOM) []model.File {
	results := make([]model.File, 0)
	artifacts := s.Artifacts

	for _, coordinates := range s.AllCoordinates() {
		var metadata *source.FileMetadata
		if metadataForLocation, exists := artifacts.FileMetadata[coordinates]; exists {
			metadata = &metadataForLocation
		}

		var digests []file.Digest
		if digestsForLocation, exists := artifacts.FileDigests[coordinates]; exists {
			digests = digestsForLocation
		}

		// TODO: add file classifications (?) and content as a snippet

		var comment string
		if coordinates.FileSystemID != "" {
			comment = fmt.Sprintf("layerID: %s", coordinates.FileSystemID)
		}

		results = append(results, model.File{
			Item: model.Item{
				Element: model.Element{
					SPDXID:  model.ElementID(coordinates.ID()).String(),
					Comment: comment,
				},
				// required, no attempt made to determine license information
				LicenseConcluded: "NOASSERTION",
			},
			Checksums: toFileChecksums(digests),
			FileName:  coordinates.RealPath,
			FileTypes: toFileTypes(metadata),
		})
	}

	// sort by real path then virtual path to ensure the result is stable across multiple runs
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].FileName == results[j].FileName {
			return results[i].SPDXID < results[j].SPDXID
		}
		return results[i].FileName < results[j].FileName
	})
	return results
}

func toFileChecksums(digests []file.Digest) (checksums []model.Checksum) {
	for _, digest := range digests {
		checksums = append(checksums, model.Checksum{
			Algorithm:     toChecksumAlgorithm(digest.Algorithm),
			ChecksumValue: digest.Value,
		})
	}
	return checksums
}

func toChecksumAlgorithm(algorithm string) string {
	// basically, we need an uppercase version of our algorithm:
	// https://github.com/spdx/spdx-spec/blob/development/v2.2.2/schemas/spdx-schema.json#L165
	return strings.ToUpper(algorithm)
}

func toFileTypes(metadata *source.FileMetadata) (ty []string) {
	if metadata == nil {
		return nil
	}

	mimeTypePrefix := strings.Split(metadata.MIMEType, "/")[0]
	switch mimeTypePrefix {
	case "image":
		ty = append(ty, string(spdxhelpers.ImageFileType))
	case "video":
		ty = append(ty, string(spdxhelpers.VideoFileType))
	case "application":
		ty = append(ty, string(spdxhelpers.ApplicationFileType))
	case "text":
		ty = append(ty, string(spdxhelpers.TextFileType))
	case "audio":
		ty = append(ty, string(spdxhelpers.AudioFileType))
	}

	if internal.IsExecutable(metadata.MIMEType) {
		ty = append(ty, string(spdxhelpers.BinaryFileType))
	}

	if internal.IsArchive(metadata.MIMEType) {
		ty = append(ty, string(spdxhelpers.ArchiveFileType))
	}

	// TODO: add support for source, spdx, and documentation file types
	if len(ty) == 0 {
		ty = append(ty, string(spdxhelpers.OtherFileType))
	}

	return ty
}

func toRelationships(relationships []artifact.Relationship) []model.Relationship {
	result := []model.Relationship{}
	for _, r := range relationships {
		exists, relationshipType, comment := lookupRelationship(r.Type)
		if !exists {
			log.Warnf("unable to convert relationship from SPDX 2.2 JSON, dropping: %+v", r)
			continue
		}

		rel := model.Relationship{
			SpdxElementID:    model.ElementID(r.From.ID()).String(),
			RelationshipType: relationshipType,
			Comment:          comment,
		}

		// if this relationship contains an external document ref, we need to use DocElementID instead of ElementID
		valid, err := isValidExternalRelationshipDocument(r)
		if err != nil {
			log.Warnf("dropping relationship %v: %w", rel, err)
			continue
		}
		if valid {
			rel.RelatedSpdxElement = model.DocElementID(r.To.ID()).String()
		} else {
			rel.RelatedSpdxElement = model.ElementID(r.To.ID()).String()
		}

		result = append(result, rel)
	}
	return result
}

func lookupRelationship(ty artifact.RelationshipType) (bool, spdxhelpers.RelationshipType, string) {
	switch ty {
	case artifact.ContainsRelationship:
		return true, spdxhelpers.ContainsRelationship, ""
	case artifact.OwnershipByFileOverlapRelationship:
		return true, spdxhelpers.OtherRelationship, fmt.Sprintf("%s: indicates that the parent package claims ownership of a child package since the parent metadata indicates overlap with a location that a cataloger found the child package by", ty)
	case artifact.DependencyOfRelationship:
		return true, spdxhelpers.DependencyOfRelationship, ""
	case artifact.DescribedByRelationship:
		return true, spdxhelpers.DescribedByRelationship, ""
	}
	return false, "", ""
}