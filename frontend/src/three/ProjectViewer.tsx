import { Suspense } from 'react';
import { Canvas } from '@react-three/fiber';
import { OrbitControls, Environment, Float, ContactShadows } from '@react-three/drei';
import type { ModelId } from '../data/projects';
import ProjectModel from './models/ProjectModel';
import { useReducedMotion, useIsMobile } from '../lib/hooks';

/** Standalone interactive viewer for a single project model (detail pages). */
export default function ProjectViewer({ id, color }: { id: ModelId; color: string }) {
  const reduced = useReducedMotion();
  const mobile = useIsMobile();

  return (
    <Canvas
      shadows={!mobile}
      dpr={mobile ? 1 : [1, 1.75]}
      camera={{ position: [2.4, 1.4, 3], fov: 40 }}
      gl={{ antialias: !mobile, powerPreference: 'high-performance' }}
    >
      <color attach="background" args={['#16033c']} />
      <ambientLight intensity={0.4} />
      <directionalLight position={[4, 6, 3]} intensity={1.2} castShadow={!mobile} />
      <pointLight position={[-3, 2, 2]} intensity={16} color={color} distance={10} />

      <Suspense fallback={null}>
        <Float
          speed={reduced ? 0 : 1.2}
          rotationIntensity={reduced ? 0 : 0.4}
          floatIntensity={reduced ? 0 : 0.6}
        >
          <ProjectModel id={id} color={color} animate={!reduced} />
        </Float>
        <Environment preset="city" />
        {!mobile && (
          <ContactShadows position={[0, -1, 0]} opacity={0.5} scale={8} blur={2.5} far={4} />
        )}
      </Suspense>

      <OrbitControls
        enablePan={false}
        enableZoom={!mobile}
        minDistance={2}
        maxDistance={6}
        autoRotate={!reduced}
        autoRotateSpeed={0.6}
        maxPolarAngle={Math.PI / 1.7}
      />
    </Canvas>
  );
}
