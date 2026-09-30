import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/**
 * Maps an invoice API failure to Spanish copy. The backend `message` is only
 * appended for the validation codes where it says what is wrong; everything
 * else falls back to the generic save error (session, server, network).
 */
export function describeInvoiceError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "unknown_client":
        return "El cliente no existe en Configuración. Revisa la lista de clientes.";
      case "invalid_invoice":
        return `Los datos no son válidos: ${err.message}`;
      case "xml_or_uuid_required":
        return "Sube el XML del CFDI o escribe el UUID para marcar la factura como emitida.";
      case "invalid_uuid":
        return "El UUID no tiene el formato correcto: 8-4-4-4-12 caracteres hexadecimales.";
      case "invalid_document":
        return `El archivo no es válido: ${err.message}`;
      case "invalid_cfdi":
        return "El XML no es un CFDI válido.";
      case "cfdi_not_stamped":
        return "El XML no está timbrado: no trae el TimbreFiscalDigital del SAT.";
      case "uuid_mismatch":
        return "El UUID del XML no coincide con el UUID de la factura.";
      case "duplicate_uuid":
        return "Ese UUID ya está registrado en otra factura.";
      case "invoice_cancelled":
        return "La factura está cancelada y ya no admite cambios.";
      case "invoice_declared":
        return "La factura está incluida en una declaración registrada y no se puede cancelar. Elimina primero el registro de esa declaración (solo si su pago sigue pendiente) y vuelve a intentar.";
      case "invoice_already_issued":
        return "La factura ya fue marcada como emitida. Para cambiar sus archivos usa la sección de documentos.";
      case "invoice_not_issued":
        return "Primero marca la factura como emitida.";
      case "invoice_state_changed":
        return "La factura cambió de estado mientras la editabas. Actualiza e intenta de nuevo.";
      case "request_too_large":
        return "Los archivos son demasiado grandes. El XML puede pesar hasta 1 MB y el PDF hasta 10 MB.";
      case "storage_unavailable":
        return "El almacenamiento de documentos no está disponible. Intenta de nuevo en unos minutos.";
    }
    if (err.status === 413) return "Los archivos son demasiado grandes. El XML puede pesar hasta 1 MB y el PDF hasta 10 MB.";
    if (err.status === 404) return "La factura o el documento ya no existe. Actualiza la lista e intenta de nuevo.";
  }
  return describeSaveError(err);
}
