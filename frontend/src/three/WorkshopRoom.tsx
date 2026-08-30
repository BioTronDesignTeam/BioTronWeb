import { useMemo } from 'react';
import { PROJECTS } from '../data/projects';
import { useScene } from '../lib/store';
import Hotspot from './Hotspot';
import ProjectModel from './models/ProjectModel';

const cssVar = (name: string) =>
  getComputedStyle(document.documentElement).getPropertyValue(name).trim() || '#160b6c';

interface RoomProps {
  reduced?: boolean;
  mobile?: boolean;
}

/**
 * Procedural placeholder for the isometric workshop room.
 *
 * ── ROOM SWAP POINT ──────────────────────────────────────────────────────────
 * Replace this body with a Blender-authored room loaded via
 * `useGLTF('/models/room.glb')`. Keep the project models mounted at the same
 * PROJECTS[].anchor coordinates and the About board near [-3.2, 1.8, -3.2] so
 * the camera path (three/CameraPath.ts) and popups keep working unchanged.
 * ────────────────────────────────────────────────────────────────────────────
 */
export default function WorkshopRoom({ reduced = false }: RoomProps) {
  const activeStop = useScene((s) => s.activeStop);
  const accent = useMemo(() => cssVar('--accent'), []);

  return (
    <group>
      {/* ---------- Shell ---------- */}
      {/* Floor */}
      <mesh rotation={[-Math.PI / 2, 0, 0]} position={[0, -0.86, -1]} receiveShadow>
        <planeGeometry args={[20, 16]} />
        <meshStandardMaterial color="#0b1117" metalness={0.2} roughness={0.9} />
      </mesh>
      {/* Floor grid glow lines */}
      <gridHelper
        args={[20, 20, accent, '#16212b']}
        position={[0, -0.85, -1]}
      />
      {/* Back wall */}
      <mesh position={[0, 2.5, -7]} receiveShadow>
        <boxGeometry args={[20, 7, 0.3]} />
        <meshStandardMaterial color="#0a1015" metalness={0.1} roughness={1} />
      </mesh>
      {/* Left wall */}
      <mesh position={[-9.8, 2.5, -1]} rotation={[0, Math.PI / 2, 0]} receiveShadow>
        <boxGeometry args={[12, 7, 0.3]} />
        <meshStandardMaterial color="#0a1015" metalness={0.1} roughness={1} />
      </mesh>

      {/* Pegboard on back wall */}
      <mesh position={[1.5, 3, -6.82]}>
        <boxGeometry args={[5, 2.2, 0.06]} />
        <meshStandardMaterial color="#101922" metalness={0.3} roughness={0.7} />
      </mesh>

      {/* ---------- Benches under each project ---------- */}
      {PROJECTS.map((p) => (
        <Bench key={p.slug} x={p.anchor[0]} z={p.anchor[2]} />
      ))}

      {/* Shelf on back wall */}
      <mesh position={[-5, 4, -6.7]} castShadow>
        <boxGeometry args={[3, 0.1, 0.6]} />
        <meshStandardMaterial color="#1a232c" metalness={0.4} roughness={0.6} />
      </mesh>

      {/* ---------- About board (wall-mounted, interactive) ---------- */}
      <Hotspot
        stop="about"
        position={[-3.2, 1.9, -3.2]}
        radius={1.1}
        active={activeStop === 'about'}
        color={accent}
        reduced={reduced}
      >
        <group rotation={[0, 0, 0]}>
          {/* board */}
          <mesh castShadow>
            <boxGeometry args={[1.9, 1.3, 0.08]} />
            <meshStandardMaterial color="#111a22" metalness={0.2} roughness={0.6} />
          </mesh>
          {/* frame glow */}
          <mesh position={[0, 0, 0.045]}>
            <boxGeometry args={[1.98, 1.38, 0.02]} />
            <meshBasicMaterial color={accent} transparent opacity={0.25} toneMapped={false} />
          </mesh>
          {/* pinned cards */}
          {[
            [-0.5, 0.3],
            [0.45, 0.35],
            [-0.35, -0.3],
            [0.5, -0.25],
          ].map(([x, y], i) => (
            <mesh key={i} position={[x, y, 0.07]} rotation={[0, 0, (i - 1.5) * 0.06]}>
              <boxGeometry args={[0.6, 0.42, 0.01]} />
              <meshStandardMaterial color="#ffffff" emissive={accent} emissiveIntensity={0.05} />
            </mesh>
          ))}
        </group>
      </Hotspot>

      {/* ---------- Project models (interactive) ---------- */}
      {PROJECTS.map((p) => (
        <Hotspot
          key={p.slug}
          stop={p.modelId}
          position={p.anchor}
          radius={1}
          active={activeStop === p.modelId}
          color={p.accentHex}
          reduced={reduced}
        >
          <ProjectModel id={p.modelId} color={p.accentHex} animate={!reduced} />
        </Hotspot>
      ))}
    </group>
  );
}

function Bench({ x, z }: { x: number; z: number }) {
  return (
    <group position={[x, -0.86, z]}>
      {/* top */}
      <mesh position={[0, 0.62, 0]} castShadow receiveShadow>
        <boxGeometry args={[2.4, 0.12, 1.4]} />
        <meshStandardMaterial color="#15202a" metalness={0.3} roughness={0.7} />
      </mesh>
      {/* legs */}
      {[
        [-1.05, -0.55],
        [1.05, -0.55],
        [-1.05, 0.55],
        [1.05, 0.55],
      ].map(([lx, lz], i) => (
        <mesh key={i} position={[lx, 0.28, lz]}>
          <boxGeometry args={[0.1, 0.66, 0.1]} />
          <meshStandardMaterial color="#0e1620" metalness={0.5} roughness={0.5} />
        </mesh>
      ))}
    </group>
  );
}
