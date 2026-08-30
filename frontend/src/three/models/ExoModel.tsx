import { useRef } from 'react';
import { useFrame } from '@react-three/fiber';
import { Group } from 'three';
import type { ModelProps } from './types';

/**
 * Stylized powered exoskeleton leg linkage built from primitives:
 * thigh + shank struts joined by glowing actuated hip/knee joints.
 * Swap for a real SolidWorks-exported .glb via the ProjectModel registry.
 */
export default function ExoModel({ color, animate = true }: ModelProps) {
  const root = useRef<Group>(null);
  const knee = useRef<Group>(null);

  useFrame((state) => {
    if (!animate) return;
    const t = state.clock.elapsedTime;
    // Gentle gait-like articulation at the knee + slow idle sway.
    if (knee.current) knee.current.rotation.x = 0.35 + Math.sin(t * 1.1) * 0.25;
    if (root.current) root.current.rotation.y = Math.sin(t * 0.4) * 0.12;
  });

  const metal = (emissive = 0.25) => (
    <meshStandardMaterial
      color="#5a6b76"
      metalness={0.85}
      roughness={0.35}
      emissive={color}
      emissiveIntensity={emissive}
    />
  );

  return (
    <group ref={root} dispose={null}>
      {/* Hip mount */}
      <mesh position={[0, 1.05, 0]} castShadow>
        <boxGeometry args={[0.55, 0.3, 0.4]} />
        {metal(0.15)}
      </mesh>

      {/* Hip joint (glowing) */}
      <mesh position={[0, 0.9, 0]} rotation={[Math.PI / 2, 0, 0]}>
        <torusGeometry args={[0.16, 0.07, 16, 32]} />
        <meshStandardMaterial color={color} emissive={color} emissiveIntensity={2.2} toneMapped={false} />
      </mesh>

      {/* Thigh strut */}
      <mesh position={[0, 0.55, 0.02]} castShadow>
        <cylinderGeometry args={[0.08, 0.1, 0.7, 16]} />
        {metal()}
      </mesh>
      <mesh position={[0.12, 0.55, 0.02]} castShadow>
        <cylinderGeometry args={[0.04, 0.04, 0.66, 12]} />
        {metal(0.4)}
      </mesh>

      {/* Knee group (articulated) */}
      <group ref={knee} position={[0, 0.2, 0]}>
        {/* Knee joint */}
        <mesh rotation={[Math.PI / 2, 0, 0]}>
          <torusGeometry args={[0.14, 0.06, 16, 32]} />
          <meshStandardMaterial color={color} emissive={color} emissiveIntensity={2.2} toneMapped={false} />
        </mesh>
        {/* Shank strut */}
        <mesh position={[0, -0.4, 0.05]} castShadow>
          <cylinderGeometry args={[0.07, 0.06, 0.7, 16]} />
          {metal()}
        </mesh>
        {/* Foot plate */}
        <mesh position={[0, -0.78, 0.12]} rotation={[0.2, 0, 0]} castShadow>
          <boxGeometry args={[0.26, 0.05, 0.5]} />
          {metal(0.15)}
        </mesh>
      </group>
    </group>
  );
}
