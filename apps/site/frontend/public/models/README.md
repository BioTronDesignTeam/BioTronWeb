# 3D Models: swapping placeholders for real SolidWorks parts

The site ships with **procedural placeholder geometry** so the full experience
(camera path, hotspots, popups, click-to-jump) works today. Everything is built
so that replacing a placeholder with a real model is a localized change.

## 1. Export from SolidWorks → glTF/GLB (web-ready)

SolidWorks does not export glTF directly. Use one of:

- **Blender** (free): SolidWorks → export `STEP`/`STL` → import into Blender →
  `File ▸ Export ▸ glTF 2.0 (.glb)`. Decimate / apply materials first.
- **CAD Exchanger** or **Blender + STEPper add-on** for clean STEP import.

Keep models **light**: aim for < 150k triangles and a single `.glb` per project.
Apply a dark metallic material; the scene lighting + bloom do the rest.

## 2. Drop files here

```
public/models/
  exo.glb          # EXO exoskeleton
  emg.glb          # EMG Fabric
  enable.glb       # e-NABLE hand
  room.glb         # (optional) full Blender workshop room
```

## 3. Swap a project model

Edit `src/three/models/ProjectModel.tsx`. Add a GLB-loading component:

```tsx
import { useGLTF } from '@react-three/drei';
function ExoGLB({ color }: ModelProps) {
  const { scene } = useGLTF('/models/exo.glb');
  return <primitive object={scene} />;
}
// then in REGISTRY: exo: ExoGLB,
```

Nothing else changes. Anchors (`PROJECTS[].anchor` in `src/data/projects.ts`),
hotspots, the camera path and popups all stay the same. Call
`useGLTF.preload('/models/exo.glb')` for snappier loads.

## 4. Swap the whole room

Author the isometric workshop in Blender and export `room.glb`. In
`src/three/WorkshopRoom.tsx`, replace the procedural body with:

```tsx
const { scene } = useGLTF('/models/room.glb');
return <primitive object={scene} />;
```

Keep the three model anchors and the About board near their current world
coordinates (see `src/three/CameraPath.ts`) so the camera keyframes still frame
each stop. Adjust `KEYFRAMES` only if you reposition things.
```
