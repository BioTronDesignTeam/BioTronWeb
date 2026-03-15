import { BatteryStatus, Header, EmergencyStop, ParameterTuning, MotorControl, MCUStatus} from "./components";

export default function Home() {
  return (
    <div>
      <Header />
      <main className="bg-sky-200 h-screen p-8">
        <BatteryStatus/>
        <MCUStatus/>
      </main>
    </div>
  );
}
