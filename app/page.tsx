import { BatteryStatus, Header, EmergencyStop, ParameterTuning, MotorStatus, MCUStatus} from "./components";

export default function Home() {
  return (
    <div className="bg-sky-200 min-h-screen p-4 items-center gap-6">
      {/* Header */}
      <Header />

      {/* Main Layout */}
      <main className="bg-sky-200 h-screen p-8 flex flex-col">
          <div className="flex gap-4">
            <BatteryStatus/>
            <MCUStatus/>
            <MotorStatus
              title="Left Motor Status"
              className="w-[350px] h-[380px]"
            />
            <MotorStatus
              title="Right Motor Status"
              className="w-[350px] h-[380px]"
            />
          </div>
          <div className="flex gap-4 p-8">
            <ParameterTuning />
            <EmergencyStop />
          </div>
      </main>
    </div>
  );
}