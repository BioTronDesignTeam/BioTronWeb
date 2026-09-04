import { useEffect, useMemo, useRef } from 'react';
import { useFrame } from '@react-three/fiber';
import {
  DoubleSide,
  Group,
  Material,
  Mesh,
  MeshBasicMaterial,
  MeshDepthMaterial,
  MeshStandardMaterial,
  PlaneGeometry,
  RGBADepthPacking,
} from 'three';
import type { ModelProps } from './types';

/**
 * The weave's resting wave, evaluated on the GPU. Kept in one string so the
 * lit panel, the wireframe overlay, and the shadow pass stay in step.
 */
const WAVE_FUNCTION = `
  uniform float uWaveTime;
  float biotronWaveZ(vec3 p, float t) {
    return sin(p.x * 3.0 + t * 1.5) * 0.08 + cos(p.y * 3.0 + t * 1.2) * 0.08;
  }
`;

/** Analytic normal of the wave, so lighting matches the displaced surface. */
const WAVE_NORMAL = `
  vec3 objectNormal = normalize(vec3(
    -0.24 * cos(position.x * 3.0 + uWaveTime * 1.5),
     0.24 * sin(position.y * 3.0 + uWaveTime * 1.2),
     1.0
  ));
`;

const WAVE_POSITION = `
  vec3 transformed = vec3(position.x, position.y, biotronWaveZ(position, uWaveTime));
`;

type WaveUniform = { uWaveTime: { value: number } };

/** Moves the panel's vertex motion into the vertex shader. */
function applyWave(material: Material, uniforms: WaveUniform, lit: boolean) {
  material.onBeforeCompile = (shader) => {
    shader.uniforms.uWaveTime = uniforms.uWaveTime;
    shader.vertexShader = WAVE_FUNCTION + shader.vertexShader;
    if (lit) {
      shader.vertexShader = shader.vertexShader.replace('#include <beginnormal_vertex>', WAVE_NORMAL);
    }
    shader.vertexShader = shader.vertexShader.replace('#include <begin_vertex>', WAVE_POSITION);
  };
}

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

  const uniforms = useMemo<WaveUniform>(() => ({ uWaveTime: { value: 0 } }), []);

  const clothMaterial = useMemo(() => {
    const material = new MeshStandardMaterial({
      color: '#241a3a',
      metalness: 0.1,
      roughness: 0.8,
      emissive: color,
      emissiveIntensity: 0.18,
      side: DoubleSide,
    });
    applyWave(material, uniforms, true);
    return material;
  }, [color, uniforms]);

  const weaveMaterial = useMemo(() => {
    const material = new MeshBasicMaterial({
      color,
      wireframe: true,
      transparent: true,
      opacity: 0.18,
      toneMapped: false,
    });
    applyWave(material, uniforms, false);
    return material;
  }, [color, uniforms]);

  // The shadow pass compiles its own program, so it needs the same displacement
  // or the panel would cast the shadow of a flat plane.
  const depthMaterial = useMemo(() => {
    const material = new MeshDepthMaterial({ depthPacking: RGBADepthPacking });
    applyWave(material, uniforms, false);
    return material;
  }, [uniforms]);

  useEffect(
    () => () => {
      clothMaterial.dispose();
      weaveMaterial.dispose();
      depthMaterial.dispose();
    },
    [clothMaterial, weaveMaterial, depthMaterial],
  );

  useEffect(() => () => geo.dispose(), [geo]);

  // Electrode node positions on a 3x3 grid across the panel.
  const nodes = useMemo(() => {
    const pts: [number, number][] = [];
    for (let i = -1; i <= 1; i++) for (let j = -1; j <= 1; j++) pts.push([i * 0.5, j * 0.5]);
    return pts;
  }, []);

  useFrame((state) => {
    const t = animate ? state.clock.elapsedTime : 0;
    uniforms.uWaveTime.value = t;

    if (pulse.current && animate) {
      pulse.current.position.x = ((Math.sin(t * 0.8) * 0.5 + 0.5) * 1.6 - 0.8);
    }
    if (root.current && animate) root.current.rotation.y = Math.sin(t * 0.35) * 0.18;
  });

  return (
    <group ref={root} rotation={[-0.35, 0, 0]} dispose={null}>
      {/* Fabric panel */}
      <mesh
        ref={cloth}
        geometry={geo}
        material={clothMaterial}
        customDepthMaterial={depthMaterial}
        castShadow
      />

      {/* Woven grid overlay */}
      <mesh geometry={geo} material={weaveMaterial} />

      {/* Electrode nodes */}
      {nodes.map(([x, y]) => (
        <mesh key={`${x}:${y}`} position={[x, y, 0.06]}>
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
