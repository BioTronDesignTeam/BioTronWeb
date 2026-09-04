import { useRef, useState } from 'react';
import { useFrame, useThree } from '@react-three/fiber';
import { Vector3 } from 'three';
import { useScene } from '../lib/store';
import { sampleCamera, stopAtProgress } from './CameraPath';

/** Scratch vector reused every frame for the parallax-offset camera target. */
const _desiredPos = new Vector3();

/**
 * Bridges scroll progress (from the store) to the camera: samples the camera
 * path, eases toward it, and adds a subtle pointer parallax. Also publishes the
 * active stop so DOM popups can react.
 */
export default function Rig({ reduced = false }: { reduced?: boolean }) {
  const camera = useThree((s) => s.camera);
  // Lazy initializers: these vectors are mutated in place every frame, so they
  // must be built once and never rebuilt during a render.
  const [targetPos] = useState(() => new Vector3(7.5, 6.5, 9.5));
  const [targetLook] = useState(() => new Vector3(0, 0.8, -1.5));
  const [currentLook] = useState(() => new Vector3(0, 0.8, -1.5));
  const lastStop = useRef<string | null>('about');

  useFrame((_, delta) => {
    const { progress, pointer, setActiveStop } = useScene.getState();

    sampleCamera(progress, targetPos, targetLook);

    // Subtle parallax from pointer (skipped under reduced motion).
    const px = reduced ? 0 : pointer.x * 0.5;
    const py = reduced ? 0 : pointer.y * 0.3;

    const lerp = reduced ? 1 : Math.min(1, delta * 3.5);
    _desiredPos.set(targetPos.x + px, targetPos.y + py, targetPos.z);
    camera.position.lerp(_desiredPos, lerp);
    currentLook.lerp(targetLook, lerp);
    camera.lookAt(currentLook);

    const stop = stopAtProgress(progress);
    if (stop !== lastStop.current) {
      lastStop.current = stop;
      setActiveStop(stop);
    }
  });

  return null;
}
