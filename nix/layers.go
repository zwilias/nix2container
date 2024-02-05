package nix

import (
	_ "crypto/sha256"
	_ "crypto/sha512"
	"reflect"

	"github.com/nlewo/nix2container/types"
	godigest "github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

func getPaths(storePaths []string, parents []types.Layer, rewrites []types.RewritePath, exclude string, permPaths []types.PermPath) types.Paths {
	var paths types.Paths
	for _, p := range storePaths {
		path := types.Path{
			Path: p,
		}
		var pathOptions types.PathOptions
		hasPathOptions := false
		var perms []types.Perm
		for _, perm := range permPaths {
			if p == perm.Path {
				hasPathOptions = true
				perms = append(perms, types.Perm{
					Regex: perm.Regex,
					Mode:  perm.Mode,
					Uid:   perm.Uid,
					Gid:   perm.Gid,
					Uname: perm.Uname,
					Gname: perm.Gname,
				})
			}
		}
		if perms != nil {
			pathOptions.Perms = perms
		}
		for _, rewrite := range rewrites {
			if p == rewrite.Path {
				hasPathOptions = true
				pathOptions.Rewrite = types.Rewrite{
					Regex: rewrite.Regex,
					Repl:  rewrite.Repl,
				}
			}
		}
		if hasPathOptions {
			path.Options = &pathOptions
		}
		if p == exclude {
			logrus.Infof("Excluding path %s from layer", p)
			continue
		}
		if isPathInLayers(parents, path) {
			logrus.Infof("Excluding path %s because already present in a parent layer", p)
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

// If tarDirectory is not an empty string, the tar layer is written to
// the disk. This is useful for layer containing non reproducible
// store paths.
func newLayers(paths types.Paths, tarDirectory string, maxLayers int) (layers []types.Layer, err error) {
	offset := 0
	for offset < len(paths) {
		max := offset + 1
		if offset == maxLayers-1 {
			max = len(paths)
		}
		layerPaths := paths[offset:max]
		layerPath := ""
		var digest godigest.Digest
		var size int64
		if tarDirectory == "" {
			logrus.Infof("Calculating digest for %d paths", len(layerPaths))
			digest, size, err = TarPathsSum(layerPaths)
			logrus.Info("Calculated digest!")
		} else {
			layerPath, digest, size, err = TarPathsWrite(paths, tarDirectory)
		}
		if err != nil {
			return layers, err
		}
		logrus.Infof("Adding %d paths to layer (size:%d digest:%s)", len(layerPaths), size, digest.String())
		layer := types.Layer{
			Digest:    digest.String(),
			DiffIDs:   digest.String(),
			Size:      size,
			Paths:     layerPaths,
			MediaType: v1.MediaTypeImageLayer,
		}
		if tarDirectory != "" {
			// TODO: we should use v1.MediaTypeImageLayerGzip instead
			layer.MediaType = v1.MediaTypeImageLayer
			layer.LayerPath = layerPath
		}

		layers = append(layers, layer)

		offset = max
	}
	return layers, nil
}

func NewLayers(storePaths []string, maxLayers int, parents []types.Layer, rewrites []types.RewritePath, exclude string, perms []types.PermPath) ([]types.Layer, error) {
	paths := getPaths(storePaths, parents, rewrites, exclude, perms)
	return newLayers(paths, "", maxLayers)
}

func NewLayersNonReproducible(storePaths []string, maxLayers int, tarDirectory string, parents []types.Layer, rewrites []types.RewritePath, exclude string, perms []types.PermPath) (layers []types.Layer, err error) {
	paths := getPaths(storePaths, parents, rewrites, exclude, perms)
	return newLayers(paths, tarDirectory, maxLayers)
}

func isPathInLayers(layers []types.Layer, path types.Path) bool {
	for _, layer := range layers {
		for _, p := range layer.Paths {
			if reflect.DeepEqual(p, path) {
				return true
			}
		}
	}
	return false
}
