import type { Bill, PayResult } from "../../api/bills";
import { Modal } from "../../components/Modal";
import { PayBillForm } from "./PayBillForm";

interface Props {
  bill: Bill;
  /** Expense category names to choose from. */
  categories: string[];
  onClose: () => void;
  onPaid: (result: PayResult) => void;
}

/** Registers the payment of a bill's pending occurrence as an expense (see PayBillForm). */
export function PayDialog({ bill, categories, onClose, onPaid }: Props) {
  return (
    <Modal title={`Pagar ${bill.name}`} onClose={onClose} highlight>
      <PayBillForm bill={bill} categories={categories} onCancel={onClose} onPaid={onPaid} />
    </Modal>
  );
}
