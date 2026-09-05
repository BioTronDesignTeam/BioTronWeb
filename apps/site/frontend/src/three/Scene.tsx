import { Suspense, useEffect, useState } from 'react';
import { Canvas } from '@react-three/fiber';
import {
  AdaptiveDpr,
  Environment,
  PerformanceMonitor,
  ContactShadows,
} from '@react-three/drei';
import { EffectComposer, Bloom, Vignette } from '@react-three/postprocessing';
import WorkshopRoom from './WorkshopRoom';
import Rig from './Rig';
import { useScene } from '../lib/store';
import { useReducedMotion, useIsMobile } from '../lib/hooks';

function SceneReady() {
  const setReady = useScene((s) => s.setReady);
  useEffect(() => {
    setReady(true);
    return () => setReady(false);
  }, [setReady]);
  return null;
}

/**
 * Persistent full-screen WebGL canvas living behind the DOM content. The camera
 * is driven by Rig from scroll progress; postprocessing and DPR scale down on
 * weaker hardware / mobile, and are trimmed entirely under reduced motion.
 */
export default function Scene() {
  const reduced = useReducedMotion();
  const mobile = useIsMobile();
  const setPointer = useScene((s) => s.setPointer);
  const [dpr, setDpr] = useState<number>(mobile ? 1 : 1.5);

  const usePost = !reduced; // bloom/vignette off under reduced motion

  return (
    <div
      style={{ position: 'fixed', inset: 0, zIndex: 0 }}
      aria-hidden="true"
      onPointerMove={(e) => {
        const x = (e.clientX / window.innerWidth) * 2 - 1;
        const y = -((e.clientY / window.innerHeight) * 2 - 1);
        setPointer(x, y);
      }}
    >
      <Canvas
        shadows={!mobile}
        dpr={dpr}
        camera={{ position: [7.5, 6.5, 9.5], fov: 38, near: 0.1, far: 100 }}
        gl={{ antialias: !mobile, powerPreference: 'high-performance' }}
      >
        <color attach="background" args={['#070b0e']} />
        <fog attach="fog" args={['#070b0e', 14, 30]} />

        <PerformanceMonitor
          onDecline={() => setDpr(1)}
          onIncline={() => setDpr(mobile ? 1 : 1.5)}
        />
        <AdaptiveDpr pixelated />

        {/* Lighting */}
        <ambientLight intensity={0.35} />
        <directionalLight
          position={[5, 8, 4]}
          intensity={1.1}
          castShadow={!mobile}
          shadow-mapSize={[1024, 1024]}
        />
        <pointLight position={[-4, 3, 2]} intensity={20} color="#160b6c" distance={12} />
        <pointLight position={[4, 2, 1]} intensity={14} color="#3050b0" distance={12} />

        <Suspense fallback={null}>
          <WorkshopRoom reduced={reduced} mobile={mobile} />
          <Environment preset="warehouse" />
          {!mobile && (
            <ContactShadows
              position={[0, -0.85, -1]}
              opacity={0.5}
              scale={20}
              blur={2.5}
              far={6}
            />
          )}
          <SceneReady />
        </Suspense>

        <Rig reduced={reduced} />

        {usePost && (
          <EffectComposer enableNormalPass={false}>
            <Bloom
              intensity={mobile ? 0.5 : 0.9}
              luminanceThreshold={0.6}
              luminanceSmoothing={0.3}
              mipmapBlur
            />
            <Vignette eskil={false} offset={0.25} darkness={0.7} />
          </EffectComposer>
        )}
      </Canvas>
    </div>
  );
}
