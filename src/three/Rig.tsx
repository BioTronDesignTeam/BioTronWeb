import { useRef } from 'react';
import { useFrame, useThree } from '@react-three/fiber';
import { Vector3 } from 'three';
import { useScene } from '../lib/store';
import { sampleCamera, stopAtProgress } from './CameraPath';

/**
 * Bridges scroll progress (from the store) to the camera: samples the camera
 * path, eases toward it, and adds a subtle pointer parallax. Also publishes the
 * active stop so DOM popups can react.
 */
export default function Rig({ reduced = false }: { reduced?: boolean }) {
  const camera = useThree((s) => s.camera);
  const targetPos = useRef(new Vector3(7.5, 6.5, 9.5));
  const targetLook = useRef(new Vector3(0, 0.8, -1.5));
  const currentLook = useRef(new Vector3(0, 0.8, -1.5));
  const lastStop = useRef<string | null>('about');

  useFrame((_, delta) => {
    const { progress, pointer, setActiveStop } = useScene.getState();

    sampleCamera(progress, targetPos.current, targetLook.current);

    // Subtle parallax from pointer (skipped under reduced motion).
    const px = reduced ? 0 : pointer.x * 0.5;
    const py = reduced ? 0 : pointer.y * 0.3;

    const lerp = reduced ? 1 : Math.min(1, delta * 3.5);
    camera.position.lerp(
      targetPos.current.clone().add(new Vector3(px, py, 0)),
      lerp,
    );
    currentLook.current.lerp(targetLook.current, lerp);
    camera.lookAt(currentLook.current);

    const stop = stopAtProgress(progress);
    if (stop !== lastStop.current) {
      lastStop.current = stop;
      setActiveStop(stop);
    }
  });

  return null;
}
