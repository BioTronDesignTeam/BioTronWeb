import { useRef } from 'react';
import { useFrame } from '@react-three/fiber';
import { Group } from 'three';
import type { ModelProps } from './types';

/**
 * Stylized 3D-printed prosthetic hand: a palm block with four fingers and a
 * thumb that gently curl in a grasp cycle. Printed-plastic look + accent joints.
 */
export default function ENableModel({ color, animate = true }: ModelProps) {
  const root = useRef<Group>(null);
  const fingers = useRef<Group[]>([]);
  const thumb = useRef<Group>(null);

  useFrame((state) => {
    const t = animate ? state.clock.elapsedTime : 0;
    const grasp = (Math.sin(t * 1.3) * 0.5 + 0.5) * 0.9; // 0..0.9
    fingers.current.forEach((f, i) => {
      if (f) f.rotation.x = grasp + i * 0.04;
    });
    if (thumb.current) thumb.current.rotation.z = -0.5 - grasp * 0.4;
    if (root.current && animate) root.current.rotation.y = Math.sin(t * 0.4) * 0.2;
  });

  const printed = (emissive = 0.05) => (
    <meshStandardMaterial color="#d9d2cb" metalness={0.05} roughness={0.7} emissive={color} emissiveIntensity={emissive} />
  );
  const joint = (
    <meshStandardMaterial color={color} emissive={color} emissiveIntensity={2} toneMapped={false} />
  );

  const Finger = ({ x, refIndex, len = 0.5 }: { x: number; refIndex: number; len?: number }) => (
    <group
      position={[x, 0.35, 0]}
      ref={(el) => {
        if (el) fingers.current[refIndex] = el;
      }}
    >
      {/* proximal */}
      <mesh position={[0, len / 2, 0]} castShadow>
        <capsuleGeometry args={[0.05, len, 6, 12]} />
        {printed()}
      </mesh>
      {/* knuckle joint */}
      <mesh position={[0, 0, 0]}>
        <sphereGeometry args={[0.06, 12, 12]} />
        {joint}
      </mesh>
      {/* distal */}
      <group position={[0, len, 0]} rotation={[0.3, 0, 0]}>
        <mesh position={[0, 0.18, 0]} castShadow>
          <capsuleGeometry args={[0.045, 0.32, 6, 12]} />
          {printed()}
        </mesh>
      </group>
    </group>
  );

  return (
    <group ref={root} rotation={[0.2, 0, 0]} dispose={null}>
      {/* Palm */}
      <mesh position={[0, 0, 0]} castShadow>
        <boxGeometry args={[0.7, 0.45, 0.22]} />
        {printed(0.08)}
      </mesh>
      {/* Wrist cuff */}
      <mesh position={[0, -0.42, 0]} castShadow>
        <cylinderGeometry args={[0.26, 0.3, 0.4, 20]} />
        {printed()}
      </mesh>
      <mesh position={[0, -0.24, 0.02]} rotation={[Math.PI / 2, 0, 0]}>
        <torusGeometry args={[0.2, 0.03, 12, 24]} />
        {joint}
      </mesh>

      {/* Fingers */}
      <Finger x={-0.24} refIndex={0} len={0.42} />
      <Finger x={-0.08} refIndex={1} len={0.52} />
      <Finger x={0.08} refIndex={2} len={0.5} />
      <Finger x={0.24} refIndex={3} len={0.4} />

      {/* Thumb */}
      <group ref={thumb} position={[-0.34, -0.05, 0.05]} rotation={[0, 0, -0.5]}>
        <mesh position={[-0.18, 0, 0]} rotation={[0, 0, Math.PI / 2]} castShadow>
          <capsuleGeometry args={[0.055, 0.34, 6, 12]} />
          {printed()}
        </mesh>
        <mesh position={[0, 0, 0]}>
          <sphereGeometry args={[0.06, 12, 12]} />
          {joint}
        </mesh>
      </group>
    </group>
  );
}
