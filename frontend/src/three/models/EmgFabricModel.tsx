import { useMemo, useRef } from 'react';
import { useFrame } from '@react-three/fiber';
import { Group, Mesh, PlaneGeometry, BufferAttribute } from 'three';
import type { ModelProps } from './types';

/**
 * EMG smart-textile: a softly waving fabric panel with reusable electrode nodes
 * and a glowing signal pulse traveling across the weave.
 */
export default function EmgFabricModel({ color, animate = true }: ModelProps) {
  const root = useRef<Group>(null);
  const cloth = useRef<Mesh>(null);
  const pulse = useRef<Mesh>(null);

  const segs = 24;
  const geo = useMemo(() => new PlaneGeometry(1.6, 1.6, segs, segs), []);
  const base = useMemo(() => Float32Array.from(geo.attributes.position.array), [geo]);

  // Electrode node positions on a 3x3 grid across the panel.
  const nodes = useMemo(() => {
    const pts: [number, number][] = [];
    for (let i = -1; i <= 1; i++) for (let j = -1; j <= 1; j++) pts.push([i * 0.5, j * 0.5]);
    return pts;
  }, []);

  useFrame((state) => {
    const t = animate ? state.clock.elapsedTime : 0;
    const pos = geo.attributes.position as BufferAttribute;
    for (let i = 0; i < pos.count; i++) {
      const x = base[i * 3];
      const y = base[i * 3 + 1];
      const z = Math.sin(x * 3 + t * 1.5) * 0.08 + Math.cos(y * 3 + t * 1.2) * 0.08;
      pos.setZ(i, z);
    }
    pos.needsUpdate = true;
    geo.computeVertexNormals();

    if (pulse.current && animate) {
      pulse.current.position.x = ((Math.sin(t * 0.8) * 0.5 + 0.5) * 1.6 - 0.8);
    }
    if (root.current && animate) root.current.rotation.y = Math.sin(t * 0.35) * 0.18;
  });

  return (
    <group ref={root} rotation={[-0.35, 0, 0]} dispose={null}>
      {/* Fabric panel */}
      <mesh ref={cloth} geometry={geo} castShadow>
        <meshStandardMaterial
          color="#241a3a"
          metalness={0.1}
          roughness={0.8}
          emissive={color}
          emissiveIntensity={0.18}
          side={2}
        />
      </mesh>

      {/* Woven grid overlay */}
      <mesh geometry={geo}>
        <meshBasicMaterial color={color} wireframe transparent opacity={0.18} toneMapped={false} />
      </mesh>

      {/* Electrode nodes */}
      {nodes.map(([x, y], i) => (
        <mesh key={i} position={[x, y, 0.06]}>
          <sphereGeometry args={[0.05, 16, 16]} />
          <meshStandardMaterial color={color} emissive={color} emissiveIntensity={2.4} toneMapped={false} />
        </mesh>
      ))}

      {/* Traveling signal pulse */}
      <mesh ref={pulse} position={[0, 0, 0.08]}>
        <boxGeometry args={[0.04, 1.6, 0.02]} />
        <meshBasicMaterial color={color} transparent opacity={0.7} toneMapped={false} />
      </mesh>
    </group>
  );
}
