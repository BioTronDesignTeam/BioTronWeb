"use client" // add this to use react hook

import React, { useState } from "react"

const BatteryStatus = () => {
  const [Highvoltage, setHighVoltage] = useState<number>(1)
  const [Highcurrent, setHighCurrent] = useState<number>(2)
  const [Medvoltage, setMedVoltage] = useState<number>(3)
  const [Medcurrent, setMedCurrent] = useState<number>(4)
  const [Lowvoltage, setLowVoltage] = useState<number>(5)
  const [Lowcurrent, setLowCurrent] = useState<number>(6)
  
  //TODO: change actual conditions for red yellow green
  const getColour = () => {
    if (Highvoltage+Medvoltage+Lowvoltage > 4) return "bg-red-500"
    if (Highvoltage+Medvoltage+Lowvoltage > 2) return "bg-yellow-400"
    return "bg-green-500"
  }
  return (
    <div className="w-[350px] rounded-2xl bg-white py-[16px] px-[16px] border-[3px] border-black">
      <div className="flex justify-between items-center">
        <h2 className="text-black font-bold text-2xl">Battery Status</h2>
        <span className={`h-4 w-4  rounded-full ${getColour()}`}></span> 
      </div>
      <div className="flex flex-col gap-[8px]">
        <hr className="border-black"></hr>
        <Status title="High Power" voltage={Highvoltage} current={Highcurrent}></Status>
        <hr className="border-black"></hr>
        <Status title="Medium Power" voltage={Medvoltage} current={Medcurrent}></Status>
        <hr className="border-black"></hr>
        <Status title="Low Power" voltage={Lowvoltage} current={Lowcurrent}></Status>
      </div>

    </div>

  )
}

type StatusProps = {
  title: string
  voltage: number
  current: number
}
const Status = ({title, voltage, current} : StatusProps) => {
  // change conditions here based on real numbers
  const getCircleColour = () => {
    if (voltage > 4) return "bg-red-500"
    if (voltage > 2) return "bg-yellow-400"
    return "bg-green-500"
  }

  const getBgColour = () => {
      if (voltage > 4) return "bg-red-200"
      if (voltage > 2) return "bg-yellow-100"
      return "bg-green-200"
  }

  return (
    <div>
      <div className="flex flex-row justify-between items-center">
        <h3 className="text-black text-xl">{title}</h3>
        <span className={`h-4 w-4 rounded-full ${getCircleColour()}`}></span>
      </div>
      <div className="flex justify-between gap-4">
        <div className={`flex flex-col w-[50%] justify-center items-center border-[1px] border-black rounded-xl ${getBgColour()}`}>
          <p className="text-black text-[13px]">Voltage</p>
          <h2 className="text-black text-[30px]">{voltage}</h2>
        </div>
        <div className={`flex flex-col w-[50%] justify-center items-center border-[1px] border-black rounded-xl ${getBgColour()}`}>
          <p className="text-black text-[13px]">Current</p>
          <h2 className="text-black text-[30px]">{current}</h2>
        </div>
      </div>
    </div>
  )
}

export default BatteryStatus
