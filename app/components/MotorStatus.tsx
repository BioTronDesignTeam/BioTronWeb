"use client";

import React from "react";

interface MotorStatusProps {
  title: string;
  className?: string;
}

export default function MotorStatus({ title, className }: MotorStatusProps) {
  return (
    <div
      className={`bg-white text-black border-3 border-black rounded-xl p-3 flex flex-col ${className || ""}`}
    >
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-gray-400">
        <h2 className="text-[22px] font-semibold">{title}</h2>
        <div className="w-3 h-3 bg-green-300 rounded-full"></div>
      </div>

      {/* Status Boxes */}
      <div className="flex flex-col gap-4 flex-1 p-3">

        <div className="bg-green-300 border border-black rounded-lg flex flex-col items-center justify-center h-[60px]">
          <p className="text-[12px]">Procedure Result</p>
          <p className="text-[20px] font-bold">SUCCESS</p>
        </div>

        <div className="bg-green-300 border border-black rounded-lg flex flex-col items-center justify-center h-[60px]">
          <p className="text-[12px]">Trajectory Status</p>
          <p className="text-[20px] font-bold">DONE</p>
        </div>

        <div className="bg-green-300 border border-black rounded-lg flex flex-col items-center justify-center h-[60px]">
          <p className="text-[12px]">Motor State</p>
          <p className="text-[20px] font-bold">IDLE</p>
        </div>

        <div className="bg-yellow-200 border border-black rounded-lg flex flex-col items-center justify-center h-[60px]">
          <p className="text-[12px]">Error</p>
          <p className="text-[16px] font-semibold">None</p>
        </div>

      </div>
    </div>
  );
}