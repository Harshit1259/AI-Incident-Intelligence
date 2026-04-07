function PaginationControls({ page, pageSize, total, hasMore, onPageChange }) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className="pagination-bar">
      <div>
        Showing page <strong>{page}</strong> of <strong>{totalPages}</strong> · total incidents <strong>{total}</strong>
      </div>
      <div className="pagination-actions">
        <button type="button" className="secondary-button" onClick={() => onPageChange(page - 1)} disabled={page <= 1}>
          Previous
        </button>
        <button type="button" className="secondary-button" onClick={() => onPageChange(page + 1)} disabled={!hasMore}>
          Next
        </button>
      </div>
    </div>
  );
}

export default PaginationControls;
