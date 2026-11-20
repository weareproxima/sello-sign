import { createAuthClient } from 'better-auth/react'
import { jwtClient } from 'better-auth/client/plugins'

/** better-auth in the browser. The OIDC provider signs in through
 * `signIn.social({ provider: <its id> })`, like any social one. */
export const authClient = createAuthClient({ plugins: [jwtClient()] })
