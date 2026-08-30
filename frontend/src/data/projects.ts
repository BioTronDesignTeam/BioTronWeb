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
      'A powered lower-body exoskeleton built to augment human strength and endurance — our entry for the Applied Collegiate Exoskeleton (ACE) International Competition 2026.',
    description: [
      'EXO is a powered lower-limb exoskeleton designed to amplify the wearer’s strength and reduce fatigue during demanding physical tasks.',
      'Actuated hip and knee joints work with a lightweight load-bearing frame and a real-time control system that reads the wearer’s intent and delivers assistive torque in sync with natural gait.',
      'The system is being engineered for the ACE International Competition 2026, where it will be benchmarked against exoskeletons from universities across the world.',
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
    team: ['Mechanical', 'Electrical', 'Controls', 'Firmware'],
  },
  {
    slug: 'emg-fabric',
    modelId: 'emg',
    name: 'EMG Fabric',
    shortName: 'EMG',
    tagline: 'Reusable electrode wearable + ML',
    blurb:
      'A reusable smart-textile electrode array that captures muscle EMG signals and decodes intent with machine learning — bringing lab-grade biosensing into everyday wearables.',
    description: [
      'EMG Fabric integrates reusable dry electrodes directly into a flexible textile, removing the cost and waste of single-use adhesive electrodes.',
      'A compact acquisition board streams multi-channel surface-EMG into a machine-learning pipeline that classifies gestures and muscle activation in real time.',
      'The platform targets rehabilitation, prosthetics control, and human–machine interfaces where comfort and repeatability matter.',
    ],
    accentVar: '--emg',
    accentHex: '#aedbfc',
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
    tagline: 'Custom 3D-printed forearm prosthetics',
    blurb:
      'Custom mechanical forearm prosthetics, 3D-printed and assembled by our students and tailored to each patient — making life-enhancing technology affordable and accessible.',
    description: [
      'Started through our collaboration with e-NABLE, a global online community, our students 3D print and assemble custom forearm prosthetics, meticulously tailored to the exact size specifications of each patient.',
      'These prosthetics use mechanical actuation for joint movement and object gripping — robust, electronics-free, and low-cost by design.',
      'Our mission is to provide an affordable, accessible solution for patients with limited financial resources, ensuring everyone has access to life-enhancing technology.',
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
      'We pioneered software for Spine Biostickers, a wearable tool designed to monitor spinal lumbar (low-back) posture in post-surgical patients, helping prevent harmful movements during recovery. Using gyroscopes attached to the patient, our system collects data and delivers real-time alerts so patients maintain safe postures for a smoother, safer recovery.',
    tags: ['Wearable', 'Gyroscope', 'Recovery'],
  },
  {
    name: 'Tetra',
    partner: 'Tetra',
    blurb:
      'In partnership with the non-profit Tetra, we designed and fabricated customized medical devices for the specific needs of real patients. Each project began with in-depth consultations to understand the unique challenges a patient faces, followed by development, rigorous testing, and delivery of a tailored solution by our engineering students — making a tangible difference for those who need it most.',
    tags: ['Assistive Devices', 'Custom', 'Patient-centered'],
  },
];

export interface SubTeam {
  name: string;
  blurb: string;
}

export const SUBTEAMS: SubTeam[] = [
  { name: 'Mechanical', blurb: 'CAD, structural design, actuation, and rapid prototyping.' },
  { name: 'Electrical', blurb: 'PCB design, power systems, and embedded hardware.' },
  { name: 'Firmware & Controls', blurb: 'Real-time control loops, sensor fusion, and embedded software.' },
  { name: 'Software & ML', blurb: 'Signal processing, machine learning, and tooling.' },
  { name: 'Outreach & Operations', blurb: 'Community partnerships, recruitment, sponsorship, and events.' },
];

export const TEAM_STATS = [
  { value: 40, suffix: '+', label: 'Active members' },
  { value: 3, suffix: '', label: 'Live projects' },
  { value: 5, suffix: '', label: 'Sub-teams' },
  { value: 2021, suffix: '', label: 'Founded', plain: true },
];

export const CONTACT = {
  email: 'biotron@uwaterloo.ca',
  facebook: 'https://www.facebook.com/uwbiotron',
  org: 'University of Waterloo Biomechatronics Design Team',
};

const JOIN_EMAIL_BODY = `Hi Biotron team,

Name:
Program and year:
Areas of interest:
What I would like to build or learn:
`;

export const JOIN_EMAIL_HREF = `mailto:${CONTACT.email}?subject=${encodeURIComponent(
  'Joining Biotron',
)}&body=${encodeURIComponent(JOIN_EMAIL_BODY)}`;

export function getProject(slug: string): Project | undefined {
  return PROJECTS.find((p) => p.slug === slug);
}
