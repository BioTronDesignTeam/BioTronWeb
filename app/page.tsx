"use client";

import Header from "./components/Header";
import DummyWebsocketDisplay from "./components/DummyWebsocketDisplay";
import MotorStatus from "./components/MotorStatus";

export default function Home() {
  return (
    <div className="bg-sky-200 min-h-screen p-4 flex flex-col items-center gap-6">

      {/* Header */}
      <Header />

      {/* Main Layout */}
      <div className="flex w-full justify-center gap-12 flex-wrap">

        {/* Right Side: Motor Status (STACKED VERTICALLY) */}
        <div className="flex flex-col gap-4">
          <MotorStatus
            title="Left Motor Status"
            className="w-[350px] h-[380px]"
          />
          <MotorStatus
            title="Right Motor Status"
            className="w-[350px] h-[380px]"
          />
        </div>

      </div>

      {/* Bottom Row */}
      <div className="flex gap-8 w-full justify-center mt-6 flex-wrap">
        <div className="w-[350px]">
          <DummyWebsocketDisplay />
        </div>
      </div>

    </div>
  );
}