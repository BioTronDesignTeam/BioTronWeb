import type { Vector3Tuple } from 'three';

export type ModelId = 'exo' | 'emg' | 'enable';

export interface ProjectSpec {
  label: string;
  value: string;
}

export interface Project {
  slug: string;
  modelId: ModelId;
  name: string;
  shortName: string;
  tagline: string;
  blurb: string;
  description: string[];
  accentVar: string; // CSS var name, e.g. '--exo'
  accentHex: string;
  status: string;
  specs: ProjectSpec[];
  /** Anchor position of the model inside the workshop room (world units). */
  anchor: Vector3Tuple;
  team: string[];
}

export const PROJECTS: Project[] = [
  {
    slug: 'exo',
    modelId: 'exo',
    name: 'EXO',
    shortName: 'EXO',
    tagline: 'Powered lower-body exoskeleton',
    blurb:
      'A powered lower-body exoskeleton that supports human strength and endurance. We are building it for ACE 2026.',
    description: [
      'EXO helps wearers handle demanding physical tasks with less fatigue.',
      'Actuators at the hips and knees work with a lightweight frame. The control system reads the wearer’s intent and adds torque in step with their gait.',
      'At the 2026 Applied Collegiate Exoskeleton (ACE) International Competition, university teams will test their systems side by side.',
    ],
    accentVar: '--exo',
    accentHex: '#3050b0',
    status: 'In development · ACE 2026',
    specs: [
      { label: 'Actuated DOF', value: '4' },
      { label: 'Target assist', value: '40 Nm' },
      { label: 'Control loop', value: '1 kHz' },
      { label: 'Competition', value: 'ACE 2026' },
    ],
    anchor: [-4.2, 0.4, -1.5],
    team: ['Mechanical', 'Electrical', 'Software'],
  },
  {
    slug: 'emg-fabric',
    modelId: 'emg',
    name: 'EMG Fabric',
    shortName: 'EMG',
    tagline: 'Reusable EMG wearable with machine learning',
    blurb:
      'A reusable textile electrode array that records muscle signals and uses machine learning to read movement intent. It brings lab-grade sensing into a wearable.',
    description: [
      'EMG Fabric embeds reusable dry electrodes in a flexible textile. It avoids the cost and waste of disposable adhesive electrodes.',
      'A compact board streams eight channels of surface EMG data to a model that classifies gestures and muscle activity in real time.',
      'The platform supports rehabilitation, prosthetic control, and human-machine interfaces that must stay comfortable and consistent.',
    ],
    accentVar: '--emg',
    accentHex: '#160b6c',
    status: 'Active research',
    specs: [
      { label: 'Channels', value: '8' },
      { label: 'Sample rate', value: '2 kHz' },
      { label: 'Electrode', value: 'Reusable' },
      { label: 'Decoding', value: 'ML' },
    ],
    anchor: [0, 0.6, -3.4],
    team: ['Textiles', 'Signal Processing', 'Machine Learning', 'PCB'],
  },
  {
    slug: 'e-nable',
    modelId: 'enable',
    name: 'e-NABLE',
    shortName: 'e-NABLE',
    tagline: 'Custom 3D-printed forearm prostheses',
    blurb:
      'Students 3D-print and assemble custom forearm prostheses for each recipient. The designs keep assistive technology practical and affordable.',
    description: [
      'Through e-NABLE, a global volunteer community, our students build forearm prostheses to each recipient’s measurements.',
      'Body movement drives the joints and grip. The design is durable, affordable, and needs no electronics.',
      'We aim to give more people access to assistive devices that fit their needs and budgets.',
    ],
    accentVar: '--enable',
    accentHex: '#ffffff',
    status: 'Ongoing · Community',
    specs: [
      { label: 'Device', value: 'Forearm' },
      { label: 'Fit', value: 'Custom' },
      { label: 'Actuation', value: 'Mechanical' },
      { label: 'Process', value: 'FDM' },
    ],
    anchor: [4.2, 0.4, -1.5],
    team: ['CAD', 'Additive Mfg', 'Outreach'],
  },
];

export interface PastProject {
  name: string;
  partner?: string;
  blurb: string;
  tags: string[];
}

export const PAST_PROJECTS: PastProject[] = [
  {
    name: 'Spine Biostickers',
    blurb:
      'Spine Biostickers tracked lower-back posture after surgery. Gyroscopes measured movement, and our software warned patients when they moved beyond safe limits.',
    tags: ['Wearable', 'Gyroscope', 'Recovery'],
  },
  {
    name: 'Tetra',
    partner: 'Tetra',
    blurb:
      'Working with the nonprofit Tetra, we built custom assistive devices for individual clients. We met with each client, designed a solution, tested it, and delivered the finished device.',
    tags: ['Assistive Devices', 'Custom', 'Client-led'],
  },
];

export interface SubTeam {
  name: string;
  blurb: string;
}

export const SUBTEAMS: SubTeam[] = [
  { name: 'Mechanical', blurb: 'CAD, structural design, actuation, and rapid prototyping.' },
  { name: 'Electrical', blurb: 'PCB design, power systems, and embedded hardware.' },
  {
    name: 'Software',
    blurb: 'Firmware, controls, signal processing, machine learning, and team tooling.',
  },
  { name: 'Outreach & Operations', blurb: 'Community partnerships, recruitment, sponsorship, and events.' },
];

export const TEAM_STATS = [
  { value: 40, suffix: '+', label: 'Active members' },
  { value: 3, suffix: '', label: 'Live projects' },
  { value: 4, suffix: '', label: 'Sub-teams' },
  { value: 2021, suffix: '', label: 'Founded', plain: true },
];

export const CONTACT = {
  email: 'biotron@uwaterloo.ca',
  instagram: 'https://www.instagram.com/uwaterloo_biotron/',
  discord: 'https://discord.gg/YBNN5ThRAA',
  org: 'University of Waterloo Biomechatronics Design Team',
};

export function getProject(slug: string): Project | undefined {
  return PROJECTS.find((p) => p.slug === slug);
}
