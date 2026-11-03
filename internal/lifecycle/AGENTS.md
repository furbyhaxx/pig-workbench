# lifecycle

Session-owned cancellation and request admission. No SDK or feature dependencies.
- `Stop` closes admission before cancelling and joining admitted work.
- Every successful `Admit` must release; cancellation alone does not release admission.
