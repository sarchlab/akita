# Akita Monitoring Tool

Akita Monitoring Tool is a toolset allow user to monitor the simulation status during the simulation is running. It also provides simple solutions that users to interact with the simulation.

## Use

If you want to develop a simulator and want it to support real-time monitoring, you can initiate the monitor in your configuration code. You also need to register the simulator engine with the `RegisterEngine` function of the `Monitor` struct and all the components that you want to monitor with the `RegisterComponent` function of the`Monitor` struct.

Before the simulation starts, call the `StartServer` function of the monitor. A web server will be hosted and the port will be printed to standard output.

You can use a web browser to access the monitoring tool hosted at the given port.

## Develop

1. Make sure you have NodeJS and npm installed.
2. In the web folder, type `npm install` to install dependencies.
3. In the web folder, run `npm run watch` to use webpack to build static assets.
4. Set environment variable `AKITA_MONITOR_DEV` to the `true` to enable the development. In development mode, your modification to the frontend code will be reflected without recompiling the simulation. 
