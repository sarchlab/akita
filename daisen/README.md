# Daisen

Daisen is the visualization tool for Akita. 

## How to use the visualization tool

### Collect Trace

If MGPUSim is used, collecting traces is as simple as adding a command line argument `-trace-vis`. By default, a trace file in `sqlite3` format with a random name will appear in the working directory. However, users can specify to use CSV format by adding `-trace-vis-db=csv` as a command line argument. The filename of the trace file can also be specified by adding `-trace-vis-db-file=[filename w/o extension name]` command line argument.

If you are developing a new simulator, you need to instrument your simulator with the `tracing` APIs. Please refer to the `tracing` APIs in [github.com/sarchlab/akita/tracing](../tracing) for more details. Then, a DB tracer will need to be attached to the components that may generate visualization traces. 

### Build Server

In the `github.com/sarchlab/akita/daisen` directory, run `go build`. The `daisen` executable will be generated.

### Start Server
Run `./daisen -[trace-format] [trace_file]`. The trace format can be `sqlite3` or `csv`. The trace file is the path to the trace file. 

## Development

The regular start server method always uses the production build of the frontend. If you want to develop the frontend, you can run `npm run dev` in the `github.com/sarchlab/akita/daisen/static` directory. This will start a development server for the frontend. By default, the vite.js development server will listen to port 5173.

Then, you can run `./daisen -[trace-format] [trace_file]` to start the API server. Make sure your API server is listening on port 3001. 

Finally, in your browser, open `localhost:5173`. You should be able to see the visualization tool. The vite.js server is very powerful as it supports hot reloading. So, you can make changes to the frontend code and see the changes immediately in the browser.

## How to use the visualization tool

Please watch the video below for a demo of Daisen

[![Daisen Demo](http://img.youtube.com/vi/t5Ej4od4lek/0.jpg)](http://www.youtube.com/watch?v=t5Ej4od4lek "Daisen Demo")