import type { ModelId } from '../../data/projects';
import type { ModelProps } from './types';
import ExoModel from './ExoModel';
import EmgFabricModel from './EmgFabricModel';
import ENableModel from './ENableModel';

/**
 * Registry mapping a project's modelId to its 3D component.
 *
 * ── SWAP POINT ──────────────────────────────────────────────────────────────
 * To replace a procedural placeholder with a real SolidWorks-exported model:
 *   1. Export the SolidWorks part to glTF/GLB (via Blender or CAD Exchanger) and
 *      drop it in `public/models/<id>.glb` (see public/models/README.md).
 *   2. Create a component that loads it with drei's `useGLTF('/models/<id>.glb')`
 *      and matches the ModelProps signature.
 *   3. Replace the entry below. Nothing else in the app needs to change. The
 *      room anchors, hotspots, camera path and popups all stay the same.
 * ────────────────────────────────────────────────────────────────────────────
 */
const REGISTRY: Record<ModelId, (props: ModelProps) => JSX.Element> = {
  exo: ExoModel,
  emg: EmgFabricModel,
  enable: ENableModel,
};

export default function ProjectModel({ id, ...props }: ModelProps & { id: ModelId }) {
  const Model = REGISTRY[id];
  return <Model {...props} />;
}
