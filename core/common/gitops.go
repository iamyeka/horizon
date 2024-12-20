// Copyright © 2023 Horizoncd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

const (
	// fileName
	GitopsFileChart          = "Chart.yaml"
	GitopsFileApplication    = "application.yaml"
	GitopsFileTags           = "tags.yaml"
	GitopsFileSRE            = "sre/sre.yaml"
	GitopsFileBase           = "system/horizon.yaml"
	GitopsFileEnv            = "system/env.yaml"
	GitopsFileRestart        = "system/restart.yaml"
	GitopsFilePipeline       = "pipeline/pipeline.yaml"
	GitopsAppPipeline        = "pipeline.yaml"
	GitopsFilePipelineOutput = "pipeline/pipeline-output.yaml"
	GitopsFileManifest       = "manifest.yaml"

	// value namespace
	GitopsEnvValueNamespace  = "env"
	GitopsBaseValueNamespace = "horizon"

	GitopsMergeRequestStateOpen = "opened"

	GitopsGroupClusters          = "clusters"
	GitopsGroupRecyclingClusters = "recycling-clusters"

	GitopsKeyTags = "tags"
)
