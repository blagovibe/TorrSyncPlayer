export const nativeRC4 = false

export class MessageStreamEncryptor {
  constructor(_wireOrKey: unknown, _skeyHex?: unknown) {}
  startAsInitiator(_infoHash: unknown): void {}
  handleStepA2(_step2Data: unknown): void {}
  handleStepB1(_step1Data: unknown): void {}
  handleIncoming(): void {}
  handleOutgoing(): void {}
  setInfoHash(_infoHash: unknown): void {}
  generateStepA1(): Uint8Array { return new Uint8Array(0) }
  generateStepA3(_cp?: number): Uint8Array { return new Uint8Array(0) }
  generateStepB2(): Uint8Array { return new Uint8Array(0) }
  generateStepB4(_cs?: number): Uint8Array { return new Uint8Array(0) }
  getSyncPattern(): Uint8Array { return new Uint8Array(0) }
  extractInfoHashFromXor(_xorPart: unknown): string { return "" }
  encrypt(data: Uint8Array): Uint8Array { return data }
  decrypt(data: Uint8Array): Uint8Array { return data }
  get dh(): unknown { return null }
  get encryptionMethod(): unknown { return null }
  get cryptoHandshakeDone(): boolean { return true }
}
