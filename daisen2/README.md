# Daisen2 - React.js Rewrite

Daisen2 is a modern React.js rewrite of the Daisen visualization tool for Akita simulators.

## Features

- **Modern React Architecture**: Built with React 18, TypeScript, and functional components
- **Interactive Visualizations**: D3.js-powered timeline visualizations with zoom and pan
- **Component Dashboard**: Grid-based dashboard showing component metrics and widgets
- **Real-time Chat**: AI-powered chat panel for simulation analysis (Daisen Bot)
- **Responsive Design**: Bootstrap 5-based responsive UI
- **TypeScript**: Full type safety throughout the application

## Architecture

### Components
- `App.tsx` - Main application with routing
- `Navbar.tsx` - Navigation bar with chat toggle
- `Dashboard.tsx` - Component overview dashboard with pagination
- `TaskView.tsx` - Detailed task timeline visualization
- `ComponentView.tsx` - Component-specific task visualization
- `TimelineVisualization.tsx` - D3.js-based timeline chart component
- `Widget.tsx` - Individual component metric widgets
- `ChatPanel.tsx` - AI chat interface
- `StatusBar.tsx` - Status information display

### Services
- `api.ts` - API service layer for Go backend communication

### Hooks
- `useApi.ts` - Custom React hooks for data fetching

### Types
- `index.ts` - TypeScript type definitions

## Development

### Setup
```bash
# Install dependencies
npm install

# Start development server
npm run dev

# Build for production
npm run build
```

### API Integration
The React app communicates with the existing Go backend through the `/api` endpoints:
- `/api/trace?kind=Simulation` - Get simulation data
- `/api/compnames` - Get component names
- `/api/trace?component=<name>` - Get component tasks
- `/api/data?component=<name>` - Get component metrics

### Development Server
The Vite development server runs on port 5174 and proxies API requests to the Go server on port 3001.

## Key Improvements over Original Daisen

1. **Modern React Patterns**: Uses functional components, hooks, and modern React patterns
2. **Better State Management**: React state and custom hooks for data management
3. **Type Safety**: Full TypeScript integration with proper type definitions
4. **Component Architecture**: Reusable components with clear separation of concerns
5. **Improved Developer Experience**: Hot reload, TypeScript intellisense, modern tooling
6. **Responsive Design**: Better mobile and tablet support
7. **Performance**: React's virtual DOM and optimized rendering

## Usage

1. Start the Go backend server (daisen) on port 3001
2. Run the React development server: `npm run dev`
3. Open http://localhost:5174 in your browser

The application will automatically proxy API requests to the Go backend while providing a modern React-based user interface for visualization and interaction.

## Future Enhancements

- Enhanced chart types and visualizations
- Real-time data streaming
- Advanced filtering and search capabilities
- Export functionality for charts and data
- Performance optimizations for large datasets
- Custom dashboard configurations
- Dark mode support