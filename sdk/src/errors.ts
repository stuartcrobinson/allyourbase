/** Error thrown when the AYB API returns a non-2xx response. */
export class AYBError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "AYBError";
  }
}
