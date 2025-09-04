import React, { useState, useMemo } from 'react';
import { Link } from 'react-router-dom';
import { useComponentNames, useSimulations } from '../hooks/useApi';
import Widget from '../components/Widget';

const Dashboard: React.FC = () => {
  const { componentNames, loading: namesLoading, error: namesError } = useComponentNames();
  const { simulations, loading: simLoading, error: simError } = useSimulations();
  const [filter, setFilter] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const componentsPerPage = 12;

  const filteredComponents = useMemo(() => {
    return componentNames.filter(name => 
      name.toLowerCase().includes(filter.toLowerCase())
    );
  }, [componentNames, filter]);

  const totalPages = Math.ceil(filteredComponents.length / componentsPerPage);
  const startIndex = (currentPage - 1) * componentsPerPage;
  const currentComponents = filteredComponents.slice(startIndex, startIndex + componentsPerPage);

  const simulation = simulations[0]; // Use first simulation for time range

  if (namesLoading || simLoading) {
    return (
      <div className="d-flex justify-content-center align-items-center h-100">
        <div className="spinner-border" role="status">
          <span className="visually-hidden">Loading...</span>
        </div>
      </div>
    );
  }

  if (namesError || simError) {
    return (
      <div className="alert alert-danger m-3">
        Error: {namesError || simError}
      </div>
    );
  }

  return (
    <div className="h-100 d-flex flex-column">
      {/* Toolbar */}
      <div className="p-3 border-bottom">
        <div className="row align-items-center">
          <div className="col-md-6">
            <input
              type="text"
              className="form-control"
              placeholder="Filter components..."
              value={filter}
              onChange={(e) => {
                setFilter(e.target.value);
                setCurrentPage(1);
              }}
            />
          </div>
          <div className="col-md-6 text-end">
            <span className="text-muted">
              Showing {currentComponents.length} of {filteredComponents.length} components
            </span>
          </div>
        </div>
      </div>

      {/* Component Grid */}
      <div className="flex-grow-1 overflow-auto">
        {currentComponents.length === 0 ? (
          <div className="text-center py-5">
            <h5 className="text-muted">No components found</h5>
            {filter && (
              <p className="text-muted">Try adjusting your filter criteria</p>
            )}
          </div>
        ) : (
          <div className="component-grid">
            {currentComponents.map(componentName => (
              <div key={componentName} className="widget-container">
                <div className="widget-header d-flex justify-content-between align-items-center">
                  <h6 className="mb-0" title={componentName}>
                    {componentName.length > 30 ? `${componentName.substring(0, 30)}...` : componentName}
                  </h6>
                  <Link
                    to={`/component?name=${encodeURIComponent(componentName)}${
                      simulation ? `&starttime=${simulation.start_time}&endtime=${simulation.end_time}` : ''
                    }`}
                    className="btn btn-sm btn-outline-primary"
                  >
                    View
                  </Link>
                </div>
                <div className="widget-content">
                  <Widget
                    componentName={componentName}
                    startTime={simulation?.start_time}
                    endTime={simulation?.end_time}
                  />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="p-3 border-top">
          <nav>
            <ul className="pagination justify-content-center mb-0">
              <li className={`page-item ${currentPage === 1 ? 'disabled' : ''}`}>
                <button
                  className="page-link"
                  onClick={() => setCurrentPage(Math.max(1, currentPage - 1))}
                  disabled={currentPage === 1}
                >
                  Previous
                </button>
              </li>
              
              {Array.from({ length: Math.min(5, totalPages) }, (_, i) => {
                const pageNum = Math.max(1, Math.min(totalPages - 4, currentPage - 2)) + i;
                if (pageNum > totalPages) return null;
                
                return (
                  <li key={pageNum} className={`page-item ${currentPage === pageNum ? 'active' : ''}`}>
                    <button
                      className="page-link"
                      onClick={() => setCurrentPage(pageNum)}
                    >
                      {pageNum}
                    </button>
                  </li>
                );
              })}
              
              <li className={`page-item ${currentPage === totalPages ? 'disabled' : ''}`}>
                <button
                  className="page-link"
                  onClick={() => setCurrentPage(Math.min(totalPages, currentPage + 1))}
                  disabled={currentPage === totalPages}
                >
                  Next
                </button>
              </li>
            </ul>
          </nav>
        </div>
      )}
    </div>
  );
};

export default Dashboard;