# Akita Monitoring Tool

Akita Monitoring Tool is a tool that allows users to monitor the simulation status while the simulation is running. It also provides simple solutions for users to control the running simulation.

## Use

If you want to develop a simulator that supports real-time monitoring, you can initiate the monitor in your configuration code. You also need to register the simulator engine with the `RegisterEngine` function of the `Monitor` struct. Also, you need to register all the components that you want to monitor with the `RegisterComponent` function of the `Monitor` struct.

Before the simulation starts, call the `StartServer` function of the monitor. A web server will be hosted and the port will be printed to standard error output.

You can use a web browser to access the monitoring tool hosted at the given port.

## Development

1. Make sure you have NodeJS and npm installed.
2. In the web folder, type `npm install` to install dependencies.
3. In the web folder, run `npm run watch` to use Webpack to build static assets.
4. Set environment variable `AKITA_MONITOR_DEV` to the `true` to enable the development mode. In development mode, your modification to the frontend code will be reflected without recompiling the simulation. 
