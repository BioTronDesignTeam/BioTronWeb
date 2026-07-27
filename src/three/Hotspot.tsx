import { useRef, useState, type ReactNode } from 'react';
import { useFrame, type ThreeEvent } from '@react-three/fiber';
import { Group } from 'three';
import type { Stop } from '../lib/store';
import { jumpToStop } from '../lib/flythrough';

interface HotspotProps {
  stop: Stop;
  position: [number, number, number];
  /** Approx radius of the invisible click/hover collider. */
  radius?: number;
  /** Whether this stop is the currently active (focused) one. */
  active?: boolean;
  /** Accent colour for the selection ring. */
  color: string;
  children: ReactNode;
  reduced?: boolean;
}

/**
 * Wraps a 3D model with an invisible raycast collider that:
 *  - shows a pointer cursor + subtle lift on hover,
 *  - draws a glowing ground ring when active,
 *  - jumps the page scroll to this stop on click (spatial navigation).
 */
export default function Hotspot({
  stop,
  position,
  radius = 0.9,
  active = false,
  color,
  children,
  reduced = false,
}: HotspotProps) {
  const inner = useRef<Group>(null);
  const ring = useRef<Group>(null);
  const [hovered, setHovered] = useState(false);

  useFrame((state, delta) => {
    if (!inner.current) return;
    const targetLift = hovered && !reduced ? 0.12 : 0;
    inner.current.position.y += (targetLift - inner.current.position.y) * Math.min(1, delta * 8);
    if (ring.current) {
      const targetScale = active ? 1 : 0.001;
      const s = ring.current.scale.x + (targetScale - ring.current.scale.x) * Math.min(1, delta * 6);
      ring.current.scale.setScalar(s);
      ring.current.rotation.z = state.clock.elapsedTime * (reduced ? 0 : 0.4);
    }
  });

  const onOver = (e: ThreeEvent<PointerEvent>) => {
    e.stopPropagation();
    setHovered(true);
    document.body.style.cursor = 'pointer';
  };
  const onOut = () => {
    setHovered(false);
    document.body.style.cursor = '';
  };
  const onClick = (e: ThreeEvent<MouseEvent>) => {
    e.stopPropagation();
    jumpToStop(stop);
  };

  return (
    <group position={position}>
      {/* Active ground ring */}
      <group ref={ring} position={[0, -0.85, 0]} rotation={[-Math.PI / 2, 0, 0]}>
        <mesh>
          <ringGeometry args={[radius * 0.95, radius * 1.05, 48]} />
          <meshBasicMaterial color={color} transparent opacity={0.6} toneMapped={false} />
        </mesh>
        <mesh>
          <ringGeometry args={[radius * 0.55, radius * 0.6, 48]} />
          <meshBasicMaterial color={color} transparent opacity={0.3} toneMapped={false} />
        </mesh>
      </group>

      {/* Model (lifts slightly on hover) */}
      <group ref={inner}>{children}</group>

      {/* Invisible collider for hover/click */}
      <mesh
        onPointerOver={onOver}
        onPointerOut={onOut}
        onClick={onClick}
        visible={false}
      >
        <sphereGeometry args={[radius * 1.25, 12, 12]} />
      </mesh>
    </group>
  );
}
