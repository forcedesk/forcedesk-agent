<?php

namespace App\Http\Controllers;

use Illuminate\Http\Request;
use Inertia\Inertia;
use Inertia\Response;

class AuthController extends Controller
{
    /**
     * Show the login form
     */
    public function showLogin(): Response
    {
        return Inertia::render('Auth/Login');
    }

    /**
     * Handle login attempt
     */
    public function login(Request $request)
    {
        $request->validate([
            'password' => 'required|string',
        ]);

        $correctPassword = env('AGENT_ADMIN_PASSWORD');

        if (empty($correctPassword)) {
            return back()->withErrors([
                'password' => 'Admin password not configured. Please set AGENT_ADMIN_PASSWORD in your .env file.',
            ]);
        }

        if ($request->password === $correctPassword) {
            session(['agent_authenticated' => true]);
            return redirect()->route('agent-settings.index');
        }

        return back()->withErrors([
            'password' => 'Invalid password.',
        ]);
    }

    /**
     * Handle logout
     */
    public function logout()
    {
        session()->forget('agent_authenticated');
        return redirect()->route('agent.login');
    }
}
