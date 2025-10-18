# 🧠 DaisenBot (Beta)

DaisenBot is a multi-modal AI assistant integrated with **Daisen**, designed to help visualize and analyze GPU execution traces from **MGPUSim**.  
This guide walks you through collecting a GPU trace and launching Daisen with the DaisenBot extension.

---

## 🚀 Quick Start-up

### Step 1: Collect a MGPUSim GPU Execution Trace

#### (1) Clone the MGPUSim Repository
In your workspace:
```bash
git clone https://github.com/sarchlab/mgpusim.git
cd mgpusim
```

#### (2) Build a Benchmark (e.g., `fir`)
```bash
cd amd/samples/fir
go build    # Make sure Go 1.25.0 is installed and available in PATH
./fir --trace-vis --timing    # Default problem size; can modify with "-length <value>"
```

#### (3) Check the Generated Execution Trace
After running the benchmark, you should find a trace file like:
```
mgpusim/amd/samples/fir/akita_sim_xxx.sqlite3
```

---

### Step 2: Launch Daisen with DaisenBot Extension to Visualize the Trace

#### (1) Clone the Akita Repository
In your workspace:
```bash
git clone https://github.com/sarchlab/akita.git
cd akita
```

#### (2) Checkout to the DaisenBot Development Branch
```bash
git checkout ml-for-perf-analysis
```

#### (3) Prepare the `.env` Credential File
Copy and paste the **attached** `.env` file (for internal use only) to:
```
akita/daisen/.env
```
If you are already inside the `akita/` directory, you can directly create or edit it with:
```bash
vi daisen/.env
```


#### (4) Install npm Packages
```bash
cd daisen/static/
npm install
```

#### (5) Build Daisen
```bash
cd ..    # Go back to daisen/
go build
```

#### (6) Launch Daisen to Visualize the Trace
```bash
./daisen -sqlite ../../mgpusim/amd/samples/fir/akita_sim_xxx.sqlite3
```
> Replace the path above with your actual trace file path if different.

#### (7) Use DaisenBot
Once Daisen launches, click the **blue DaisenBot button** at the bottom-right corner of the web interface to start interacting with the assistant.

---

## 🧩 Notes
- Ensure **Go** and **Node.js (npm)** are installed before starting.
- This beta version and credential file `.env` is for **internal use only**.
- For any issues, please contact the DaisenBot development team: exu03@wm.edu (Enze Xu).

---

© 2025 DaisenBot Team — Internal Beta Release
